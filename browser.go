package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BrowserType represents the type of browser detected
type BrowserType int

const (
	BrowserUnknown BrowserType = iota
	BrowserChrome
	BrowserChromium
	BrowserFirefox
)

func (b BrowserType) String() string {
	switch b {
	case BrowserChrome:
		return "Chrome"
	case BrowserChromium:
		return "Chromium"
	case BrowserFirefox:
		return "Firefox"
	default:
		return "Unknown"
	}
}

// BrowserInfo contains information about a detected browser
type BrowserInfo struct {
	Type       BrowserType
	Executable string
	IsDefault  bool // true if this was detected as the system default browser
}

// detectBrowser attempts to find an installed browser
// It first tries to detect the system's default browser, then falls back to
// checking for Chrome, Chromium, and Firefox in that order
func detectBrowser() (*BrowserInfo, error) {
	// First, try to detect the default browser
	if browser := detectDefaultBrowser(); browser != nil {
		browser.IsDefault = true
		return browser, nil
	}

	// Fall back to checking browsers in order
	return detectBrowserFallback()
}

// detectDefaultBrowser attempts to find the system's default web browser
func detectDefaultBrowser() *BrowserInfo {
	if runtime.GOOS == "darwin" {
		return detectDefaultBrowserMacOS()
	}
	return detectDefaultBrowserLinux()
}

// detectDefaultBrowserLinux uses xdg-settings to find the default browser
func detectDefaultBrowserLinux() *BrowserInfo {
	// Try xdg-settings first
	out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	if err != nil {
		// Try xdg-mime as fallback
		out, err = exec.Command("xdg-mime", "query", "default", "x-scheme-handler/http").Output()
		if err != nil {
			return nil
		}
	}

	desktop := strings.TrimSpace(string(out))
	desktop = strings.ToLower(desktop)

	// Map .desktop file to browser type and find executable
	switch {
	case strings.Contains(desktop, "firefox"):
		if path, err := exec.LookPath("firefox"); err == nil {
			return &BrowserInfo{Type: BrowserFirefox, Executable: path}
		}
		if path, err := exec.LookPath("firefox-esr"); err == nil {
			return &BrowserInfo{Type: BrowserFirefox, Executable: path}
		}
	case strings.Contains(desktop, "google-chrome"):
		if path, err := exec.LookPath("google-chrome"); err == nil {
			return &BrowserInfo{Type: BrowserChrome, Executable: path}
		}
		if path, err := exec.LookPath("google-chrome-stable"); err == nil {
			return &BrowserInfo{Type: BrowserChrome, Executable: path}
		}
	case strings.Contains(desktop, "chromium"):
		if path, err := exec.LookPath("chromium"); err == nil {
			return &BrowserInfo{Type: BrowserChromium, Executable: path}
		}
		if path, err := exec.LookPath("chromium-browser"); err == nil {
			return &BrowserInfo{Type: BrowserChromium, Executable: path}
		}
	}

	return nil
}

// detectDefaultBrowserMacOS finds the default browser on macOS
func detectDefaultBrowserMacOS() *BrowserInfo {
	// Use the 'defaults' command to read the default browser bundle ID
	// from the launch services preferences
	out, err := exec.Command("defaults", "read",
		"com.apple.LaunchServices/com.apple.launchservices.secure",
		"LSHandlers").Output()
	if err != nil {
		return nil
	}

	output := strings.ToLower(string(out))

	// Look for the http handler entry and extract the bundle ID
	// The output is a plist-style array, we look for LSHandlerURLScheme = http
	// and its corresponding LSHandlerRoleAll

	// Simple heuristic: check which browser bundle ID appears after "http"
	httpIdx := strings.Index(output, `"lshandlerurlscheme" = "http"`)
	if httpIdx == -1 {
		httpIdx = strings.Index(output, `lshandlerurlscheme = http`)
	}

	if httpIdx != -1 {
		// Look in the surrounding context for the bundle ID
		searchArea := output[max(0, httpIdx-200):min(len(output), httpIdx+200)]

		switch {
		case strings.Contains(searchArea, "org.mozilla.firefox"):
			if _, err := os.Stat("/Applications/Firefox.app"); err == nil {
				return &BrowserInfo{
					Type:       BrowserFirefox,
					Executable: "/Applications/Firefox.app/Contents/MacOS/firefox",
				}
			}
		case strings.Contains(searchArea, "com.google.chrome"):
			if _, err := os.Stat("/Applications/Google Chrome.app"); err == nil {
				return &BrowserInfo{
					Type:       BrowserChrome,
					Executable: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
				}
			}
		case strings.Contains(searchArea, "org.chromium.chromium"):
			if _, err := os.Stat("/Applications/Chromium.app"); err == nil {
				return &BrowserInfo{
					Type:       BrowserChromium,
					Executable: "/Applications/Chromium.app/Contents/MacOS/Chromium",
				}
			}
		}
	}

	return nil
}

// detectBrowserFallback checks for browsers in a fixed order
func detectBrowserFallback() (*BrowserInfo, error) {
	var candidates []struct {
		browserType BrowserType
		names       []string
	}

	if runtime.GOOS == "darwin" {
		candidates = []struct {
			browserType BrowserType
			names       []string
		}{
			{BrowserChrome, []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}},
			{BrowserChromium, []string{"/Applications/Chromium.app/Contents/MacOS/Chromium"}},
			{BrowserFirefox, []string{"/Applications/Firefox.app/Contents/MacOS/firefox"}},
		}
	} else {
		// Linux
		candidates = []struct {
			browserType BrowserType
			names       []string
		}{
			{BrowserChrome, []string{"google-chrome", "google-chrome-stable"}},
			{BrowserChromium, []string{"chromium", "chromium-browser"}},
			{BrowserFirefox, []string{"firefox", "firefox-esr"}},
		}
	}

	for _, c := range candidates {
		for _, name := range c.names {
			var path string
			var err error

			if filepath.IsAbs(name) {
				// For macOS absolute paths, check directly
				if _, err := os.Stat(name); err == nil {
					path = name
				}
			} else {
				// For Linux, use LookPath
				path, err = exec.LookPath(name)
			}

			if err == nil && path != "" {
				return &BrowserInfo{
					Type:       c.browserType,
					Executable: path,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no supported browser found (looked for Chrome, Chromium, Firefox)")
}

// buildBrowserCommand builds the shell command to launch the browser with proxy settings
func buildBrowserCommand(browser *BrowserInfo, proxyAddr string) (string, error) {
	switch browser.Type {
	case BrowserChrome, BrowserChromium:
		return buildChromeCommand(browser.Executable, proxyAddr), nil
	case BrowserFirefox:
		return buildFirefoxCommand(browser.Executable, proxyAddr)
	default:
		return "", fmt.Errorf("unsupported browser type: %s", browser.Type)
	}
}

func buildChromeCommand(executable, proxyAddr string) string {
	var userDataDir string
	if runtime.GOOS == "darwin" {
		userDataDir = "$HOME/Library/Application Support/Google/Captive"
	} else {
		userDataDir = "$HOME/.google-chrome-captive"
	}

	// Quote executable if it contains spaces
	quotedExec := executable
	if strings.Contains(executable, " ") {
		quotedExec = fmt.Sprintf(`"%s"`, executable)
	}

	var cmd string
	if runtime.GOOS == "darwin" {
		// On macOS, use open command to properly launch the app
		cmd = fmt.Sprintf(`open -n -W -a "Google Chrome" --args \
    --user-data-dir="%s" \
    --proxy-server="socks5://$PROXY" \
    --proxy-bypass-list="<-loopback>" \
    --no-first-run \
    --new-window \
    --incognito \
    --no-default-browser-check \
    --no-crash-upload \
    --disable-extensions \
    --disable-sync \
    --disable-background-networking \
    --disable-client-side-phishing-detection \
    --disable-component-update \
    --disable-translate \
    --disable-web-resources \
    --safebrowsing-disable-auto-update \
    http://example.com`, userDataDir)
	} else {
		cmd = fmt.Sprintf(`%s \
    --user-data-dir="%s" \
    --proxy-server="socks5://$PROXY" \
    --proxy-bypass-list="<-loopback>" \
    --no-first-run \
    --new-window \
    --incognito \
    --no-default-browser-check \
    --no-crash-upload \
    --disable-extensions \
    --disable-sync \
    --disable-background-networking \
    --disable-client-side-phishing-detection \
    --disable-component-update \
    --disable-translate \
    --disable-web-resources \
    --safebrowsing-disable-auto-update \
    http://example.com`, quotedExec, userDataDir)
	}

	return cmd
}

func buildFirefoxCommand(executable, proxyAddr string) (string, error) {
	profileDir, err := setupFirefoxProfile(proxyAddr)
	if err != nil {
		return "", fmt.Errorf("failed to setup Firefox profile: %w", err)
	}

	// Quote executable if it contains spaces
	quotedExec := executable
	if strings.Contains(executable, " ") {
		quotedExec = fmt.Sprintf(`"%s"`, executable)
	}

	var cmd string
	if runtime.GOOS == "darwin" {
		cmd = fmt.Sprintf(`open -n -W -a "Firefox" --args \
    -profile "%s" \
    -no-remote \
    -private-window \
    http://example.com`, profileDir)
	} else {
		cmd = fmt.Sprintf(`%s \
    -profile "%s" \
    -no-remote \
    -private-window \
    http://example.com`, quotedExec, profileDir)
	}

	return cmd, nil
}

// setupFirefoxProfile creates a Firefox profile directory with proxy settings
func setupFirefoxProfile(proxyAddr string) (string, error) {
	var profileDir string
	if runtime.GOOS == "darwin" {
		profileDir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Firefox", "Captive")
	} else {
		profileDir = filepath.Join(os.Getenv("HOME"), ".mozilla", "firefox-captive")
	}

	// Create profile directory if it doesn't exist
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Parse proxy address to get host and port
	host, port := parseProxyAddr(proxyAddr)

	// Create user.js with proxy settings
	// The $PROXY env var won't be expanded here, so we use the actual proxyAddr
	userJS := fmt.Sprintf(`// Captive browser proxy settings - auto-generated
user_pref("network.proxy.type", 1);
user_pref("network.proxy.socks", "%s");
user_pref("network.proxy.socks_port", %s);
user_pref("network.proxy.socks_version", 5);
user_pref("network.proxy.socks_remote_dns", true);
user_pref("network.proxy.no_proxies_on", "localhost, 127.0.0.1");

// Disable various Firefox features for captive portal use
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("datareporting.policy.dataSubmissionEnabled", false);
user_pref("toolkit.telemetry.reportingpolicy.firstRun", false);
user_pref("browser.newtabpage.activity-stream.feeds.telemetry", false);
user_pref("browser.newtabpage.activity-stream.telemetry", false);
user_pref("browser.ping-centre.telemetry", false);
user_pref("toolkit.telemetry.enabled", false);
user_pref("toolkit.telemetry.unified", false);
user_pref("app.shield.optoutstudies.enabled", false);
user_pref("app.update.enabled", false);
user_pref("extensions.update.enabled", false);
user_pref("browser.safebrowsing.downloads.enabled", false);
user_pref("browser.safebrowsing.malware.enabled", false);
user_pref("browser.safebrowsing.phishing.enabled", false);
`, host, port)

	userJSPath := filepath.Join(profileDir, "user.js")
	if err := os.WriteFile(userJSPath, []byte(userJS), 0644); err != nil {
		return "", fmt.Errorf("failed to write user.js: %w", err)
	}

	return profileDir, nil
}

// parseProxyAddr parses a proxy address like "localhost:1666" into host and port
func parseProxyAddr(addr string) (string, string) {
	parts := strings.Split(addr, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// Default to localhost:1666 if parsing fails
	return "localhost", "1666"
}
