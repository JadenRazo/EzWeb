package auth

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
)

func GenerateTOTPSecret(username, issuer string) (*otp.Key, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	return key, dataURI, nil
}

func ValidateTOTPCode(code, secret string) bool {
	return totp.Validate(code, secret)
}

type PendingTOTPClaims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

func GeneratePendingToken(userID int, secret string) (string, error) {
	claims := PendingTOTPClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "totp_pending",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ValidatePendingToken(tokenStr, secret string) (int, error) {
	claims := &PendingTOTPClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid pending token: %w", err)
	}
	if !token.Valid {
		return 0, fmt.Errorf("pending token is not valid")
	}
	if claims.Subject != "totp_pending" {
		return 0, fmt.Errorf("token is not a TOTP pending token")
	}
	return claims.UserID, nil
}
