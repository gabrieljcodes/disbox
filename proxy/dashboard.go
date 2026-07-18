package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTemplate.Execute(w, nil)
}

func (s *Server) handleHostersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hostersTemplate.Execute(w, nil)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	redirectURI := s.baseURL + "/auth/callback"
	u, _ := url.Parse("https://discord.com/api/oauth2/authorize")
	q := u.Query()
	q.Set("client_id", s.discordClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "identify guilds.members.read")
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	redirectURI := s.baseURL + "/auth/callback"

	// 1. Exchange code for token
	data := url.Values{}
	data.Set("client_id", s.discordClientID)
	data.Set("client_secret", s.discordClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, _ := http.NewRequest("POST", "https://discord.com/api/oauth2/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("OAuth token error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil || tokenRes.AccessToken == "" {
		log.Printf("OAuth token decode error or empty token")
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	// 2. Fetch user profile
	reqUser, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	reqUser.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)

	respUser, err := client.Do(reqUser)
	if err != nil {
		log.Printf("OAuth user profile error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}
	defer respUser.Body.Close()

	var userRes struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}
	if err := json.NewDecoder(respUser.Body).Decode(&userRes); err != nil {
		log.Printf("OAuth user decode error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	avatarURL := "https://cdn.discordapp.com/embed/avatars/0.png"
	if userRes.Avatar != "" {
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", userRes.ID, userRes.Avatar)
	}

	// Guild/Role Whitelist Check
	whitelistEnabled, _ := s.store.GetAccessSettings()
	if whitelistEnabled == "true" {
		listType, err := s.store.CheckAccess(userRes.ID)
		isExplicitlyWhitelisted := (err == nil && listType == "whitelist")

		guildRolesMap := s.store.GetGuildRolesWhitelist()
		if !isExplicitlyWhitelisted && len(guildRolesMap) > 0 {
		outer:
			for guildID, requiredRoles := range guildRolesMap {
				if len(requiredRoles) == 0 {
					continue
				}
				reqMember, _ := http.NewRequest("GET", fmt.Sprintf("https://discord.com/api/users/@me/guilds/%s/member", guildID), nil)
				reqMember.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
				respMember, err := client.Do(reqMember)
				if err == nil && respMember.StatusCode == http.StatusOK {
					var memberRes struct {
						Roles []string `json:"roles"`
					}
					if err := json.NewDecoder(respMember.Body).Decode(&memberRes); err == nil {
						for _, userRole := range memberRes.Roles {
							for _, reqRole := range requiredRoles {
								if userRole == reqRole {
									// Automatically add to whitelist
									s.store.AddToAccessList(userRes.ID, userRes.Username, avatarURL, "whitelist", "AutoRoleSync")
									log.Printf("User %s added to whitelist via AutoRoleSync (Guild: %s, Role: %s)", userRes.ID, guildID, reqRole)
									break outer
								}
							}
						}
					}
					respMember.Body.Close()
				} else if err == nil {
					respMember.Body.Close()
				}
			}
		}
	}

	// 3. Check access control
	if isAllowed, _ := s.CheckAccess(userRes.ID); !isAllowed {
		log.Printf("User %s denied login by access control", userRes.ID)
		http.Redirect(w, r, "/dashboard?error=access_denied", http.StatusTemporaryRedirect)
		return
	}

	// 4. Create session
	b := make([]byte, 32)
	rand.Read(b)
	sessionToken := hex.EncodeToString(b)

	if err := s.store.SaveSession(sessionToken, userRes.ID, userRes.Username, avatarURL); err != nil {
		log.Printf("Session save error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	// Set cookie (30 days)
	http.SetCookie(w, &http.Cookie{
		Name:     "disbox_session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.baseURL, "https"),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("disbox_session")
	if err == nil {
		s.store.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "disbox_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}
