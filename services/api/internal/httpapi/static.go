package httpapi

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

// serveWeb serves files from a configured build directory. Every file is opened
// through os.Root so decoded URL paths and symlinks cannot escape the build root.
func serveWeb(w http.ResponseWriter, r *http.Request, webDistDir, authOrigin string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead || reservedWebPath(r.URL.Path) {
		writeError(w, r, domain.NotFound())
		return
	}

	name, valid := staticFileName(r.URL.Path)
	if !valid {
		writeError(w, r, domain.NotFound())
		return
	}
	root, err := os.OpenRoot(webDistDir)
	if err != nil {
		writeError(w, r, domain.Fail(500, "INTERNAL_ERROR", "The request could not be completed"))
		return
	}
	defer root.Close()

	file, err := root.Open(name)
	if err != nil || !regularFile(file) {
		if file != nil {
			_ = file.Close()
		}
		if !isSPAPath(r.URL.Path) || !acceptsHTML(r.Header.Get("Accept")) {
			writeError(w, r, domain.NotFound())
			return
		}
		name = "index.html"
		file, err = root.Open(name)
		if err != nil || !regularFile(file) {
			if file != nil {
				_ = file.Close()
			}
			writeError(w, r, domain.Fail(500, "INTERNAL_ERROR", "The request could not be completed"))
			return
		}
		w.Header().Add("Vary", "Accept")
	}
	defer file.Close()

	setStaticHeaders(w, name, authOrigin)
	info, err := file.Stat()
	if err != nil {
		writeError(w, r, domain.Fail(500, "INTERNAL_ERROR", "The request could not be completed"))
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func regularFile(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode().IsRegular()
}

func staticFileName(urlPath string) (string, bool) {
	if !strings.HasPrefix(urlPath, "/") || strings.ContainsAny(urlPath, "\\:\x00") {
		return "", false
	}
	name := strings.TrimPrefix(urlPath, "/")
	if name == "" {
		return "index.html", true
	}
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func unsafeWebPath(urlPath string) bool {
	if strings.ContainsAny(urlPath, "\\\x00") {
		return true
	}
	for _, segment := range strings.Split(strings.TrimPrefix(urlPath, "/"), "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func reservedWebPath(urlPath string) bool {
	for _, prefix := range []string{"/api", "/healthz", "/readyz"} {
		if urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/") {
			return true
		}
	}
	return false
}

func isSPAPath(urlPath string) bool {
	if urlPath == "/" {
		return true
	}
	if reservedWebPath(urlPath) || urlPath == "/assets" || strings.HasPrefix(urlPath, "/assets/") {
		return false
	}
	return path.Ext(strings.TrimSuffix(urlPath, "/")) == ""
}

func acceptsHTML(header string) bool {
	for _, item := range strings.Split(header, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil || mediaType != "text/html" {
			continue
		}
		if rawQuality, ok := params["q"]; ok {
			quality, err := strconv.ParseFloat(rawQuality, 64)
			if err != nil || quality <= 0 {
				continue
			}
		}
		return true
	}
	return false
}

func setStaticHeaders(w http.ResponseWriter, name, authOrigin string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	connect := "'self'"
	if authOrigin != "" {
		connect += " " + authOrigin
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src "+connect+"; font-src 'self' data:")
	switch {
	case name == "index.html":
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}
