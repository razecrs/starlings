package starlings

import (
	json "encoding/json/v2"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMultipartBody pins Discord's upload format: the JSON goes in a
// "payload_json" part, files in "files[N]", and the payload must carry an
// "attachments" array whose IDs match the part indices - files are silently
// dropped otherwise, which is not an error anyone would spot.
func TestMultipartBody(t *testing.T) {
	req := request{
		Body: SendData{Content: "here you go"},
		Files: []File{
			FileFromBytes("chart.png", []byte("fake-png-bytes")),
			{Name: "notes.txt", Description: "some notes", Data: strings.NewReader("hello")},
		},
	}

	body, contentType, err := multipartBody(req)
	if err != nil {
		t.Fatal(err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q", mediaType)
	}

	mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	parts := map[string]string{}
	types := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		content, err := io.ReadAll(p)
		if err != nil {
			t.Fatal(err)
		}
		parts[p.FormName()] = string(content)
		types[p.FormName()] = p.Header.Get("Content-Type")
	}

	if _, ok := parts["payload_json"]; !ok {
		t.Fatalf("no payload_json part, got parts %v", keysOf(parts))
	}
	if got := parts["files[0]"]; got != "fake-png-bytes" {
		t.Errorf("files[0] = %q", got)
	}
	if got := parts["files[1]"]; got != "hello" {
		t.Errorf("files[1] = %q", got)
	}
	if got := types["files[0]"]; got != "image/png" {
		t.Errorf("files[0] content type = %q, want image/png", got)
	}

	// The payload must keep the caller's fields and gain the attachments.
	var payload struct {
		Content     string `json:"content"`
		Attachments []struct {
			ID          int    `json:"id"`
			Filename    string `json:"filename"`
			Description string `json:"description"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(parts["payload_json"]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Content != "here you go" {
		t.Errorf("content = %q, the original body was lost", payload.Content)
	}
	if len(payload.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(payload.Attachments))
	}
	if payload.Attachments[0].ID != 0 || payload.Attachments[0].Filename != "chart.png" {
		t.Errorf("attachment 0 = %+v", payload.Attachments[0])
	}
	if payload.Attachments[1].Description != "some notes" {
		t.Errorf("description lost: %+v", payload.Attachments[1])
	}
}

// TestMultipartWithoutBody covers sending attachments with no message body.
func TestMultipartWithoutBody(t *testing.T) {
	body, contentType, err := multipartBody(request{
		Files: []File{FileFromBytes("a.txt", []byte("x"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contentType, "boundary=") {
		t.Errorf("content type has no boundary: %q", contentType)
	}
	if !strings.Contains(string(body), `"attachments"`) {
		t.Error("attachments array missing when there is no body")
	}
}

func TestMultipartRejectsEmptyFile(t *testing.T) {
	_, _, err := multipartBody(request{Files: []File{{Name: "empty.txt"}}})
	if err == nil {
		t.Fatal("a file with no data should be rejected before the request is sent")
	}
}

func TestFileFromPathIsLazyAndMultipartSpoolRewinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := FileFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Data != nil || file.path == "" {
		t.Fatalf("FileFromPath eagerly retained data: %#v", file)
	}
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	spool, size, contentType, err := multipartTempBody(request{Files: []File{file}})
	if err != nil {
		t.Fatal(err)
	}
	spoolName := spool.Name()
	t.Cleanup(func() {
		_ = spool.Close()
		_ = os.Remove(spoolName)
	})
	prepared := preparedBody{file: spool, size: size}
	var copies [][]byte
	for range 2 {
		reader, gotSize, err := prepared.reader()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if gotSize != int64(len(got)) {
			t.Fatalf("content length = %d, body = %d", gotSize, len(got))
		}
		copies = append(copies, got)
	}
	if string(copies[0]) != string(copies[1]) || !strings.Contains(string(copies[0]), "second") {
		t.Fatalf("spooled retries differ or missed disk content: %q / %q", copies[0], copies[1])
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"a.png":     "image/png",
		"a.JPG":     "image/jpeg",
		"a.gif":     "image/gif",
		"log.txt":   "text/plain",
		"data.json": "application/json",
		"thing.bin": "application/octet-stream",
		"no-ext":    "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEscapeQuotes(t *testing.T) {
	if got := escapeQuotes(`we"ird\name.png`); got != `we\"ird\\name.png` {
		t.Errorf("escapeQuotes = %q", got)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
