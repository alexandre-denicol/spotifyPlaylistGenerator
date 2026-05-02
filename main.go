package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	redirectURI  = "http://127.0.0.1:8888/callback"
	authURL      = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	apiBaseURL   = "https://api.spotify.com/v1"
	callbackAddr = "127.0.0.1:8888"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type TokenCache struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type SearchResponse struct {
	Tracks struct {
		Items []struct {
			URI  string `json:"uri"`
			Name string `json:"name"`
		} `json:"items"`
	} `json:"tracks"`
}

type CreatePlaylistResponse struct {
	ID          string `json:"id"`
	ExternalURL struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

func main() {
	genre, trackCount, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables")
	}

	accessToken, err := getAccessToken(clientID, clientSecret)
	if err != nil {
		log.Fatal(err)
	}

	trackURIs, err := searchTracksByGenre(accessToken, genre, trackCount*4)
	if err != nil {
		log.Fatal(err)
	}

	if len(trackURIs) == 0 {
		log.Fatalf("no tracks found for genre/search term: %s", genre)
	}

	shuffle(trackURIs)

	if len(trackURIs) > trackCount {
		trackURIs = trackURIs[:trackCount]
	}

	playlistName := fmt.Sprintf(
		"Random %s - %s",
		titleCase(genre),
		time.Now().Format("2006-01-02 15:04"),
	)

	playlist, err := createPlaylist(accessToken, playlistName, genre)
	if err != nil {
		log.Fatal(err)
	}

	if err := addTracksToPlaylist(accessToken, playlist.ID, trackURIs); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Playlist created successfully:")
	fmt.Println(playlist.ExternalURL.Spotify)

	if playlist.ExternalURL.Spotify != "" {
		if err := openBrowser(playlist.ExternalURL.Spotify); err != nil {
			fmt.Println("Could not open playlist automatically.")
			fmt.Println(playlist.ExternalURL.Spotify)
		}
	}
}

func parseArgs(args []string) (string, int, error) {
	if len(args) == 0 {
		return "", 0, errors.New(`usage: go run main.go "genre name" [track_count]`)
	}

	trackCount := 50

	lastArg := args[len(args)-1]
	if parsed, err := strconv.Atoi(lastArg); err == nil {
		if parsed <= 0 {
			return "", 0, errors.New("track_count must be a positive integer")
		}

		trackCount = parsed
		args = args[:len(args)-1]
	}

	genre := strings.TrimSpace(strings.Join(args, " "))
	if genre == "" {
		return "", 0, errors.New("genre cannot be empty")
	}

	return genre, trackCount, nil
}

func getAccessToken(clientID, clientSecret string) (string, error) {
	forceLogin := os.Getenv("SPOTIFY_FORCE_LOGIN") == "1"

	if !forceLogin {
		cache, cacheErr := loadTokenCache()

		if cacheErr == nil && cache.AccessToken != "" && cache.ExpiresAt > time.Now().Unix() {
			return cache.AccessToken, nil
		}

		if cacheErr == nil && cache.RefreshToken != "" {
			token, err := refreshAccessToken(clientID, clientSecret, cache.RefreshToken)
			if err != nil {
				return "", err
			}

			if err := saveRefreshedTokenCache(cache, token); err != nil {
				return "", err
			}

			return token.AccessToken, nil
		}
	}

	codeVerifier := randomString(64)
	codeChallenge := pkceChallenge(codeVerifier)
	state := randomString(32)

	code, err := getAuthorizationCode(clientID, codeChallenge, state)
	if err != nil {
		return "", err
	}

	token, err := exchangeCodeForToken(clientID, clientSecret, code, codeVerifier)
	if err != nil {
		return "", err
	}

	if err := saveTokenCache(token); err != nil {
		return "", err
	}

	return token.AccessToken, nil
}

func getAuthorizationCode(clientID, codeChallenge, expectedState string) (string, error) {
	codeChannel := make(chan string, 1)
	errorChannel := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    callbackAddr,
		Handler: mux,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		if query.Get("state") != expectedState {
			errorChannel <- errors.New("invalid OAuth state")
			return
		}

		if errMsg := query.Get("error"); errMsg != "" {
			errorChannel <- fmt.Errorf("Spotify authorization error: %s", errMsg)
			return
		}

		code := query.Get("code")
		if code == "" {
			errorChannel <- errors.New("missing authorization code")
			return
		}

		_, _ = w.Write([]byte("Spotify authorization completed. You can close this tab."))
		codeChannel <- code
	})

	scopes := []string{
		"playlist-modify-private",
		"playlist-modify-public",
	}

	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", strings.Join(scopes, " "))
	values.Set("state", expectedState)
	values.Set("code_challenge_method", "S256")
	values.Set("code_challenge", codeChallenge)

	loginURL := authURL + "?" + values.Encode()

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorChannel <- err
		}
	}()

	fmt.Println("Opening Spotify login...")

	if err := openBrowser(loginURL); err != nil {
		fmt.Println("Could not open browser automatically. Open this URL manually:")
		fmt.Println(loginURL)
	}

	select {
	case code := <-codeChannel:
		_ = server.Close()
		return code, nil
	case err := <-errorChannel:
		_ = server.Close()
		return "", err
	case <-time.After(2 * time.Minute):
		_ = server.Close()
		return "", errors.New("timeout waiting for Spotify authorization")
	}
}

func exchangeCodeForToken(clientID, clientSecret, code, codeVerifier string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	req.Header.Set("Authorization", "Basic "+basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token TokenResponse
	if err := doJSON(req, &token); err != nil {
		return nil, err
	}

	if token.AccessToken == "" {
		return nil, errors.New("Spotify returned an empty access token")
	}

	if token.RefreshToken == "" {
		return nil, errors.New("Spotify returned an empty refresh token")
	}

	return &token, nil
}

func refreshAccessToken(clientID, clientSecret, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	req.Header.Set("Authorization", "Basic "+basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token TokenResponse
	if err := doJSON(req, &token); err != nil {
		return nil, err
	}

	if token.AccessToken == "" {
		return nil, errors.New("Spotify returned an empty refreshed access token")
	}

	return &token, nil
}

func tokenCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".spotify-random-playlist-token.json"
	}

	return home + "/.spotify-random-playlist-token.json"
}

func loadTokenCache() (*TokenCache, error) {
	data, err := os.ReadFile(tokenCachePath())
	if err != nil {
		return nil, err
	}

	var cache TokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	if cache.RefreshToken == "" {
		return nil, errors.New("missing refresh token in cache")
	}

	return &cache, nil
}

func saveTokenCache(token *TokenResponse) error {
	cache := TokenCache{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second).Unix(),
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tokenCachePath(), data, 0600)
}

func saveRefreshedTokenCache(oldCache *TokenCache, token *TokenResponse) error {
	refreshToken := oldCache.RefreshToken
	if token.RefreshToken != "" {
		refreshToken = token.RefreshToken
	}

	cache := TokenCache{
		AccessToken:  token.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second).Unix(),
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tokenCachePath(), data, 0600)
}

func searchTracksByGenre(accessToken, genre string, desiredAmount int) ([]string, error) {
	var uris []string

	if desiredAmount <= 0 {
		desiredAmount = 50
	}

	searchLimit := 10
	maxOffset := 950
	query := genre

	for offset := 0; offset <= maxOffset && len(uris) < desiredAmount; offset += searchLimit {
		endpoint, err := url.Parse(apiBaseURL + "/search")
		if err != nil {
			return nil, err
		}

		params := endpoint.Query()
		params.Set("q", query)
		params.Set("type", "track")
		params.Set("limit", strconv.Itoa(searchLimit))
		params.Set("offset", strconv.Itoa(offset))
		endpoint.RawQuery = params.Encode()

		req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+accessToken)

		var search SearchResponse
		if err := doJSON(req, &search); err != nil {
			return nil, fmt.Errorf(
				"search failed at offset=%d limit=%d url=%s: %w",
				offset,
				searchLimit,
				endpoint.String(),
				err,
			)
		}

		if len(search.Tracks.Items) == 0 {
			break
		}

		for _, item := range search.Tracks.Items {
			if item.URI != "" {
				uris = append(uris, item.URI)
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	return uniqueStrings(uris), nil
}

func createPlaylist(accessToken, name, genre string) (*CreatePlaylistResponse, error) {
	body := map[string]any{
		"name":        name,
		"public":      false,
		"description": fmt.Sprintf("Random playlist generated from Spotify search term: %s", genre),
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := apiBaseURL + "/me/playlists"

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	var playlist CreatePlaylistResponse
	if err := doJSON(req, &playlist); err != nil {
		return nil, err
	}

	if playlist.ID == "" {
		return nil, errors.New("Spotify returned an empty playlist id")
	}

	return &playlist, nil
}

func addTracksToPlaylist(accessToken, playlistID string, trackURIs []string) error {
	const batchSize = 100

	for start := 0; start < len(trackURIs); start += batchSize {
		end := start + batchSize
		if end > len(trackURIs) {
			end = len(trackURIs)
		}

		body := map[string]any{
			"uris": trackURIs[start:end],
		}

		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}

		endpoint := fmt.Sprintf("%s/playlists/%s/items", apiBaseURL, url.PathEscape(playlistID))

		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")

		if err := doJSON(req, nil); err != nil {
			return err
		}
	}

	return nil
}

func doJSON(req *http.Request, target any) error {
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"Spotify API error: method=%s url=%s status=%d body=%s",
			req.Method,
			req.URL.String(),
			resp.StatusCode,
			string(body),
		)
	}

	if target == nil || len(body) == 0 {
		return nil
	}

	return json.Unmarshal(body, target)
}

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	result := make([]byte, length)

	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			panic(err)
		}

		result[i] = chars[n.Int64()]
	}

	return string(result)
}

func pkceChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func shuffle(items []string) {
	for i := len(items) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			panic(err)
		}

		j := int(n.Int64())
		items[i], items[j] = items[j], items[i]
	}
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))

	for _, item := range items {
		if seen[item] {
			continue
		}

		seen[item] = true
		result = append(result, item)
	}

	return result
}

func titleCase(value string) string {
	words := strings.Fields(strings.ToLower(value))

	for i, word := range words {
		if len(word) == 0 {
			continue
		}

		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}

	return strings.Join(words, " ")
}

func openBrowser(targetURL string) error {
	if browser := os.Getenv("BROWSER"); browser != "" {
		return exec.Command(browser, targetURL).Start()
	}

	if runtime.GOOS == "linux" {
		candidates := []string{
			"xdg-open",
			"opera",
			"google-chrome",
			"chromium",
			"chromium-browser",
			"firefox",
		}

		for _, candidate := range candidates {
			if _, err := exec.LookPath(candidate); err == nil {
				return exec.Command(candidate, targetURL).Start()
			}
		}

		return errors.New("no browser command found")
	}

	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	case "darwin":
		return exec.Command("open", targetURL).Start()
	default:
		return errors.New("unsupported platform")
	}
}
