// Package auth signs players in. Google OAuth is the real path; a dev login
// sits behind the same session cookie so everything else can be built and
// tested before a Google client ID exists.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"easygo/internal/store"
)

const (
	cookieName  = "eg_session"
	stateCookie = "eg_oauth_state"
	sessionTTL  = 30 * 24 * time.Hour
	userInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// Config is read from the environment by the caller.
type Config struct {
	Secret       []byte // signs session cookies
	BaseURL      string // public URL of this server, e.g. http://localhost:8080
	GoogleID     string
	GoogleSecret string
	// DevLogin allows name-only sign-in. Never enable on a public server.
	DevLogin bool
	// Admins are email addresses allowed to change server settings.
	Admins []string
}

type Auth struct {
	cfg    Config
	store  *store.Store
	google *oauth2.Config
	secure bool
	// UserInfoURL is overridable in tests.
	userInfoURL string
}

func New(cfg Config, st *store.Store) *Auth {
	a := &Auth{cfg: cfg, store: st, userInfoURL: userInfoURL, secure: strings.HasPrefix(cfg.BaseURL, "https://")}
	if cfg.GoogleID != "" && cfg.GoogleSecret != "" {
		a.google = &oauth2.Config{
			ClientID: cfg.GoogleID, ClientSecret: cfg.GoogleSecret,
			RedirectURL: strings.TrimRight(cfg.BaseURL, "/") + "/auth/google/callback",
			Scopes:      []string{"openid", "email", "profile"},
			Endpoint:    google.Endpoint,
		}
	}
	return a
}

// GoogleEnabled reports whether Google sign-in is configured.
func (a *Auth) GoogleEnabled() bool { return a.google != nil }

// DevEnabled reports whether name-only sign-in is allowed.
func (a *Auth) DevEnabled() bool { return a.cfg.DevLogin }

// IsAdmin reports whether u may change server settings (KataGo paths,
// dictionary downloads). With dev login on, the server is a personal one on
// this machine and every user is its owner; otherwise only listed emails.
func (a *Auth) IsAdmin(u store.User) bool {
	if a.cfg.DevLogin {
		return true
	}
	email := strings.ToLower(u.Email)
	if email == "" {
		return false
	}
	for _, adm := range a.cfg.Admins {
		if strings.ToLower(strings.TrimSpace(adm)) == email {
			return true
		}
	}
	return false
}

// Register mounts the auth routes.
func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/google/login", a.googleLogin)
	mux.HandleFunc("GET /auth/google/callback", a.googleCallback)
	mux.HandleFunc("POST /api/auth/dev-login", a.devLogin)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
}

func (a *Auth) sign(userID string, exp int64) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(userID)) + "." + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, a.cfg.Secret)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) verify(v string) (string, bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return "", false
	}
	mac := hmac.New(sha256.New, a.cfg.Secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) != 1 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	return string(id), true
}

// UserID returns the signed-in user's ID, or "" for an anonymous request.
func (a *Auth) UserID(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	id, ok := a.verify(c.Value)
	if !ok {
		return ""
	}
	return id
}

// User loads the signed-in user, if any.
func (a *Auth) User(r *http.Request) (store.User, bool) {
	id := a.UserID(r)
	if id == "" {
		return store.User{}, false
	}
	u, err := a.store.User(id)
	return u, err == nil
}

// SessionCookie is the signed cookie that signs userID in.
func (a *Auth) SessionCookie(userID string) *http.Cookie {
	exp := time.Now().Add(sessionTTL)
	return &http.Cookie{
		Name: cookieName, Value: a.sign(userID, exp.Unix()), Path: "/", Expires: exp,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	}
}

func (a *Auth) setSession(w http.ResponseWriter, userID string) {
	http.SetCookie(w, a.SessionCookie(userID))
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func (a *Auth) devLogin(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.DevLogin {
		http.Error(w, "dev login is disabled", http.StatusForbidden)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	slug := strings.Trim(slugRE.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if name == "" || slug == "" || len(name) > 40 {
		http.Error(w, "choose a name of 1 to 40 characters", http.StatusBadRequest)
		return
	}
	u := store.User{ID: "dev:" + slug, Name: name, Provider: "dev"}
	if err := a.store.UpsertUser(u); err != nil {
		http.Error(w, "could not sign in", http.StatusInternalServerError)
		return
	}
	a.setSession(w, u.ID)
	cur, _ := a.store.User(u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cur)
}

func (a *Auth) googleLogin(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		http.Error(w, "Google sign-in is not configured", http.StatusNotFound)
		return
	}
	b := make([]byte, 16)
	rand.Read(b)
	state := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/auth/google", MaxAge: 600,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, a.google.AuthCodeURL(state), http.StatusFound)
}

func (a *Auth) googleCallback(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		http.Error(w, "Google sign-in is not configured", http.StatusNotFound)
		return
	}
	c, err := r.Cookie(stateCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.URL.Query().Get("state"))) != 1 || c.Value == "" {
		http.Error(w, "sign-in expired or was tampered with; try again", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/auth/google", MaxAge: -1})
	if e := r.URL.Query().Get("error"); e != "" {
		http.Redirect(w, r, "/?signin="+e, http.StatusFound)
		return
	}
	tok, err := a.google.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Google rejected the sign-in", http.StatusBadGateway)
		return
	}
	resp, err := a.google.Client(r.Context(), tok).Get(a.userInfoURL)
	if err != nil {
		http.Error(w, "could not read your Google profile", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var info struct {
		Sub, Email, Name, Picture string
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Sub == "" {
		http.Error(w, "unexpected Google profile", http.StatusBadGateway)
		return
	}
	name := info.Name
	if name == "" {
		name = strings.SplitN(info.Email, "@", 2)[0]
	}
	u := store.User{ID: "google:" + info.Sub, Email: info.Email, Name: name, Picture: info.Picture, Provider: "google"}
	if err := a.store.UpsertUser(u); err != nil {
		http.Error(w, "could not save your account", http.StatusInternalServerError)
		return
	}
	a.setSession(w, u.ID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// NewSecret makes a random cookie-signing secret.
func NewSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("no randomness available: %v", err))
	}
	return b
}
