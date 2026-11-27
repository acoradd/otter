package resolve

import (
	"path"
	"strings"
)

// ChromeMajorVersion is the major version of Chrome stable whose User-Agent
// Otter sends by default. Update it with Chrome's releases (about every
// four weeks): only this number changes in the reduced User-Agent.
const ChromeMajorVersion = 154

// UserAgent is the reduced User-Agent of Chrome stable on the given OS.
func UserAgent(os string) string {
	platform := "X11; Linux x86_64"
	switch os {
	case "windows":
		platform = "Windows NT 10.0; Win64; x64"
	case "darwin":
		platform = "Macintosh; Intel Mac OS X 10_15_7"
	}
	return "Mozilla/5.0 (" + platform + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" +
		itoa(ChromeMajorVersion) + ".0.0.0 Safari/537.36"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// fileTypes maps file extensions to the Content-Type of a file body. A table
// of our own: mime.TypeByExtension reads system files.
var fileTypes = map[string]string{
	".json": "application/json",
	".xml":  "application/xml",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".html": "text/html",
	".htm":  "text/html",
	".js":   "text/javascript",
	".css":  "text/css",
	".yaml": "application/yaml",
	".yml":  "application/yaml",
	".pdf":  "application/pdf",
	".zip":  "application/zip",
	".gz":   "application/gzip",
	".tar":  "application/x-tar",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".wasm": "application/wasm",
}

// FileContentType is the Content-Type of a file from its extension,
// application/octet-stream when unknown.
func FileContentType(file string) string {
	if t, ok := fileTypes[strings.ToLower(path.Ext(strings.ReplaceAll(file, "\\", "/")))]; ok {
		return t
	}
	return "application/octet-stream"
}

// bodyContentType is the automatic Content-Type of a body type; computed
// tells it is completed when sending (the multipart boundary).
func bodyContentType(bodyType, file string) (value string, computed bool) {
	switch bodyType {
	case "json":
		return "application/json", false
	case "xml":
		return "application/xml", false
	case "text":
		return "text/plain", false
	case "form-urlencoded":
		return "application/x-www-form-urlencoded", false
	case "multipart":
		return "multipart/form-data", true
	case "binary":
		return FileContentType(file), false
	}
	return "", false
}

// autoHeaders are added after the folders, unless a folder defines them.
func autoHeaders(r *Request, os, file string) [][2]string {
	var list [][2]string
	if ct, _ := bodyContentType(r.BodyType, file); ct != "" {
		list = append(list, [2]string{"Content-Type", ct})
	}
	return append(list,
		[2]string{"User-Agent", UserAgent(os)},
		[2]string{"Accept", "*/*"},
		[2]string{"Accept-Encoding", "gzip, deflate, br"},
	)
}
