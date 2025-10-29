package kontakte

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// oauthClient holds the configuration for an OAuth2 client.
type oauthClient struct {
	ID          string
	Secret      string
	RedirectURI string
}

// authCodeClaims defines the JWT claims for our short-lived authorization code.
type authCodeClaims struct {
	jwt.RegisteredClaims
	ClientID string `json:"client_id"`
}

// tokenClaims defines the JWT claims for our access and refresh tokens.
type tokenClaims struct {
	jwt.RegisteredClaims
	IsRefreshToken bool `json:"is_refresh_token,omitempty"`
}

// authorizeHandler handles the /authorize endpoint of the OAuth2 flow.
func (srv *Server) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")

	client, ok := srv.oauthClients[clientID]
	if !ok || client.RedirectURI != redirectURI || responseType != "code" {
		http.Error(w, "Invalid client or request", http.StatusBadRequest)
		return
	}

	auth := srv.checkAuthCookie(r)
	if auth == nil {
		// If not logged in, redirect to login page, but tell it to come back here.
		http.Redirect(w, r, "/login?redirect="+r.URL.RequestURI(), http.StatusFound)
		return
	}

	// Generate short-lived JWT to act as authorization code
	expiry := time.Now().Add(60 * time.Second)
	claims := &authCodeClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			Subject:   auth.Subject,
		},
		ClientID: clientID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(srv.jwtSecret)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect back to the client with the code
	finalRedirectURI := client.RedirectURI + "?code=" + ss
	http.Redirect(w, r, finalRedirectURI, http.StatusFound)
}

// tokenHandler handles the /token endpoint of the OAuth2 flow.
func (srv *Server) tokenHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	grantType := r.Form.Get("grant_type")

	switch grantType {
	case "authorization_code":
		srv.handleAuthCodeGrant(w, r)
	case "refresh_token":
		srv.handleRefreshTokenGrant(w, r)
	default:
		http.Error(w, "Unsupported grant type", http.StatusBadRequest)
	}
}

func (srv *Server) handleAuthCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.Form.Get("code")
	clientID := r.Form.Get("client_id")
	clientSecret := r.Form.Get("client_secret")

	client, ok := srv.oauthClients[clientID]
	if !ok || client.Secret != clientSecret {
		http.Error(w, "Invalid client credentials", http.StatusUnauthorized)
		return
	}

	claims := &authCodeClaims{}
	token, err := jwt.ParseWithClaims(code, claims, func(token *jwt.Token) (any, error) {
		return srv.jwtSecret, nil
	})

	if err != nil || !token.Valid || claims.ClientID != clientID {
		http.Error(w, "Invalid authorization code", http.StatusBadRequest)
		return
	}

	// Issue access and refresh tokens
	accessToken, err := srv.createToken(claims.Subject, 15*time.Minute, false)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	refreshToken, err := srv.createToken(claims.Subject, 7*24*time.Hour, true)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"refresh_token": refreshToken,
	})
}

func (srv *Server) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.Form.Get("refresh_token")

	claims := &tokenClaims{}
	token, err := jwt.ParseWithClaims(refreshToken, claims, func(token *jwt.Token) (any, error) {
		return srv.jwtSecret, nil
	})

	if err != nil || !token.Valid || !claims.IsRefreshToken {
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	// Issue a new access token
	accessToken, err := srv.createToken(claims.Subject, 15*time.Minute, false)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"token_type":   "Bearer",
	})
}

func (srv *Server) createToken(subject string, validity time.Duration, isRefresh bool) (string, error) {
	expiry := time.Now().Add(validity)
	claims := &tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			Subject:   subject,
		},
		IsRefreshToken: isRefresh,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(srv.jwtSecret)
}
