package auth

import (
	"wegugin/config"

	"github.com/dgrijalva/jwt-go"
)

func ValidateToken(tokenStr string) (bool, error) {
	_, err := ExtractClaim(tokenStr)
	if err != nil {
		return false, err
	}
	return true, nil
}

func ExtractClaim(tokenStr string) (*jwt.MapClaims, error) {
	conf := config.Load()
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(conf.Token.TOKEN_KEY), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !(ok && token.Valid) {
		return nil, err
	}

	return &claims, nil
}

func GetUserIdFromToken(req string) (Id string, Role string, err error) {
	conf := config.Load()
	Token, err := jwt.Parse(req, func(token *jwt.Token) (interface{}, error) { return []byte(conf.Token.TOKEN_KEY), nil })
	if err != nil || !Token.Valid {
		return "", "", err
	}
	claims, ok := Token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", err
	}
	Id = claims["user_id"].(string)
	Role = claims["role"].(string)

	return Id, Role, nil
}
