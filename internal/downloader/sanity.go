package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SanityCheckPackage rejects files that are obviously not what was requested:
// empty or tiny files and HTML/JSON error pages that mirrors sometimes return with HTTP 200.
func SanityCheckPackage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() < 512 {
		return fmt.Errorf("файл подозрительно мал (%d Б)", st.Size())
	}

	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]
	text := strings.ToLower(strings.TrimLeft(string(head), " \t\r\n\xef\xbb\xbf"))
	if strings.HasPrefix(text, "<!doctype html") || strings.HasPrefix(text, "<html") {
		return fmt.Errorf("зеркало вернуло HTML-страницу вместо файла")
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".ipk", ".apk", ".gz":
		if text != "" && (text[0] == '{' || text[0] == '<') {
			return fmt.Errorf("получен ответ сервера вместо пакета")
		}
	}
	return nil
}
