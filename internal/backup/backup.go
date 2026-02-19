package backup

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ezweb/internal/models"
)

type BackupInfo struct {
	Name      string
	Path      string
	Size      int64
	CreatedAt time.Time
	Type      string // "database" or "site"
	SiteName  string // only for site backups
}

type Manager struct {
	backupDir string
	db        *sql.DB
	maxAge    time.Duration
}

func NewManager(backupDir string, db *sql.DB) *Manager {
	os.MkdirAll(backupDir, 0750)
	return &Manager{
		backupDir: backupDir,
		db:        db,
		maxAge:    30 * 24 * time.Hour, // 30 days retention
	}
}

// BackupDatabase creates a gzip-compressed copy of the SQLite database.
func (m *Manager) BackupDatabase(dbPath string) (*BackupInfo, error) {
	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("ezweb-db-%s.sql.gz", ts)
	outPath := filepath.Join(m.backupDir, name)

	src, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	gz, err := gzip.NewWriterLevel(dst, gzip.BestCompression)
	if err != nil {
		os.Remove(outPath)
		return nil, fmt.Errorf("gzip writer: %w", err)
	}

	if _, err := io.Copy(gz, src); err != nil {
		gz.Close()
		os.Remove(outPath)
		return nil, fmt.Errorf("copy db: %w", err)
	}
	gz.Close()

	info, _ := os.Stat(outPath)
	return &BackupInfo{
		Name:      name,
		Path:      outPath,
		Size:      info.Size(),
		CreatedAt: time.Now(),
		Type:      "database",
	}, nil
}

// BackupSite creates a tar.gz backup of a site's Docker volumes/compose dir.
func (m *Manager) BackupSite(site models.Site) (*BackupInfo, error) {
	ts := time.Now().Format("20060102-150405")
	safeName := strings.ReplaceAll(site.Domain, ".", "-")
	name := fmt.Sprintf("site-%s-%s.tar.gz", safeName, ts)
	outPath := filepath.Join(m.backupDir, name)

	var srcDir string
	if site.IsLocal && site.ComposePath != "" {
		srcDir = filepath.Dir(site.ComposePath)
	} else {
		srcDir = filepath.Join("/opt/ezweb", site.ContainerName)
	}

	if _, err := os.Stat(srcDir); err != nil {
		return nil, fmt.Errorf("source directory not found: %s", srcDir)
	}

	cmd := exec.Command("tar", "czf", outPath, "-C", filepath.Dir(srcDir), filepath.Base(srcDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(outPath)
		return nil, fmt.Errorf("tar failed: %s: %w", string(out), err)
	}

	info, _ := os.Stat(outPath)
	return &BackupInfo{
		Name:      name,
		Path:      outPath,
		Size:      info.Size(),
		CreatedAt: time.Now(),
		Type:      "site",
		SiteName:  site.Domain,
	}, nil
}

// ListBackups returns all backup files sorted by creation time (newest first).
func (m *Manager) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	var backups []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}

		bi := BackupInfo{
			Name:      e.Name(),
			Path:      filepath.Join(m.backupDir, e.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		}

		if strings.HasPrefix(e.Name(), "ezweb-db-") {
			bi.Type = "database"
		} else if strings.HasPrefix(e.Name(), "site-") {
			bi.Type = "site"
			parts := strings.SplitN(strings.TrimPrefix(e.Name(), "site-"), "-", -1)
			if len(parts) > 2 {
				bi.SiteName = strings.Join(parts[:len(parts)-2], ".")
			}
		}

		backups = append(backups, bi)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// RestoreDatabase replaces the current database file with a backup.
// The caller must ensure the database is not in active use.
func (m *Manager) RestoreDatabase(backupName, dbPath string) error {
	// Prevent path traversal
	if strings.Contains(backupName, "/") || strings.Contains(backupName, "..") {
		return fmt.Errorf("invalid backup name")
	}

	backupPath := filepath.Join(m.backupDir, backupName)
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Only allow restoring database backups
	if !strings.HasPrefix(backupName, "ezweb-db-") {
		return fmt.Errorf("can only restore database backups")
	}

	// Create a safety backup of the current database before overwriting
	safetyName := fmt.Sprintf("ezweb-db-pre-restore-%s.sql.gz", time.Now().Format("20060102-150405"))
	safetyPath := filepath.Join(m.backupDir, safetyName)

	srcFile, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open current db: %w", err)
	}

	safetyFile, err := os.Create(safetyPath)
	if err != nil {
		srcFile.Close()
		return fmt.Errorf("create safety backup: %w", err)
	}

	gz, err := gzip.NewWriterLevel(safetyFile, gzip.BestSpeed)
	if err != nil {
		srcFile.Close()
		safetyFile.Close()
		return fmt.Errorf("gzip writer: %w", err)
	}

	if _, err := io.Copy(gz, srcFile); err != nil {
		gz.Close()
		srcFile.Close()
		safetyFile.Close()
		return fmt.Errorf("safety backup copy: %w", err)
	}
	gz.Close()
	srcFile.Close()
	safetyFile.Close()

	// Decompress the backup and write to the database path
	backupFile, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer backupFile.Close()

	gzReader, err := gzip.NewReader(backupFile)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzReader.Close()

	dstFile, err := os.Create(dbPath)
	if err != nil {
		return fmt.Errorf("create db file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, gzReader); err != nil {
		return fmt.Errorf("restore copy: %w", err)
	}

	return nil
}

// DeleteBackup removes a specific backup file.
func (m *Manager) DeleteBackup(name string) error {
	// Prevent path traversal
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return fmt.Errorf("invalid backup name")
	}
	path := filepath.Join(m.backupDir, name)
	return os.Remove(path)
}

// CleanOldBackups removes backups older than the retention period.
func (m *Manager) CleanOldBackups() int {
	cutoff := time.Now().Add(-m.maxAge)
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(m.backupDir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed
}

// RunFullBackup performs a database backup and all site backups.
func (m *Manager) RunFullBackup(dbPath string) ([]BackupInfo, error) {
	var results []BackupInfo

	// Database backup
	dbBackup, err := m.BackupDatabase(dbPath)
	if err != nil {
		log.Printf("database backup failed: %v", err)
	} else {
		results = append(results, *dbBackup)
		log.Printf("database backup: %s (%d bytes)", dbBackup.Name, dbBackup.Size)
	}

	// Site backups
	sites, err := models.GetAllSites(m.db)
	if err != nil {
		return results, fmt.Errorf("list sites: %w", err)
	}

	for _, site := range sites {
		bi, err := m.BackupSite(site)
		if err != nil {
			log.Printf("backup failed for site %s: %v", site.Domain, err)
			continue
		}
		results = append(results, *bi)
		log.Printf("site backup: %s (%d bytes)", bi.Name, bi.Size)
	}

	// Clean old backups
	removed := m.CleanOldBackups()
	if removed > 0 {
		log.Printf("cleaned %d old backups", removed)
	}

	return results, nil
}

// FormatSize returns a human-readable file size.
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
