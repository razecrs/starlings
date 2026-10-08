package starlings

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// File is an attachment to upload.
//
// Name is what Discord shows and must include an extension for previews to
// work. Description becomes the image's alt text.
type File struct {
	Name        string
	Description string
	ContentType string // guessed from the name when empty
	Data        io.Reader
	path        string
}

// FileFromPath prepares a file from disk without loading it into memory. The
// caller closes nothing; Starlings opens it while constructing the upload.
func FileFromPath(path string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("starlings: opening %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return File{}, fmt.Errorf("starlings: closing %s: %w", path, err)
	}
	return File{
		Name: filepath.Base(path),
		path: path,
	}, nil
}

// FileFromBytes attaches data already in memory.
func FileFromBytes(name string, data []byte) File {
	return File{Name: name, Data: bytes.NewReader(data)}
}

// attachmentMeta is the entry Discord expects in the JSON payload for each
// uploaded file, linking a part index to its display name.
type attachmentMeta struct {
	ID          int    `json:"id"`
	Filename    string `json:"filename"`
	Description string `json:"description,omitzero"`
}

// multipartBody encodes a request as multipart/form-data.
//
// Discord's format is particular: the JSON body goes in a part literally named
// "payload_json", each file in "files[N]", and the payload must carry an
// "attachments" array matching those indices or the files are silently
// dropped.
func multipartBody(req request) (body []byte, contentType string, err error) {
	var buf bytes.Buffer
	contentType, err = writeMultipart(&buf, req)
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), contentType, nil
}

func multipartTempBody(req request) (file *os.File, size int64, contentType string, err error) {
	file, err = os.CreateTemp("", "starlings-upload-*")
	if err != nil {
		return nil, 0, "", fmt.Errorf("starlings: creating upload spool: %w", err)
	}
	clean := func() {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}
	contentType, err = writeMultipart(file, req)
	if err != nil {
		clean()
		return nil, 0, "", err
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		clean()
		return nil, 0, "", fmt.Errorf("starlings: sizing upload spool: %w", err)
	}
	return file, position, contentType, nil
}

func writeMultipart(dst io.Writer, req request) (contentType string, err error) {
	w := multipart.NewWriter(dst)

	// Merge the attachment metadata into whatever body was supplied, so the
	// caller never has to build the "attachments" array by hand.
	payload, err := withAttachments(req.Body, req.Files)
	if err != nil {
		return "", err
	}
	if err := w.WriteField("payload_json", string(payload)); err != nil {
		return "", err
	}

	for i, f := range req.Files {
		data, closeData, err := openFileData(f)
		if err != nil {
			return "", err
		}

		part, err := w.CreatePart(filePartHeader(i, f))
		if err != nil {
			closeData()
			return "", err
		}
		_, copyErr := io.Copy(part, data)
		closeData()
		if copyErr != nil {
			return "", fmt.Errorf("starlings: reading file %q: %w", f.Name, copyErr)
		}
	}

	if err := w.Close(); err != nil {
		return "", err
	}
	return w.FormDataContentType(), nil
}

func openFileData(f File) (io.Reader, func(), error) {
	if f.path != "" {
		opened, err := os.Open(f.path)
		if err != nil {
			return nil, func() {}, fmt.Errorf("starlings: opening file %q: %w", f.Name, err)
		}
		return opened, func() { _ = opened.Close() }, nil
	}
	if f.Data == nil {
		return nil, func() {}, fmt.Errorf("starlings: file %q has no data", f.Name)
	}
	return f.Data, func() {}, nil
}

// filePartHeader builds the MIME header for one file part.
func filePartHeader(i int, f File) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader, 2)
	h.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="files[%d]"; filename=%q`, i, escapeQuotes(f.Name)))

	ct := f.ContentType
	if ct == "" {
		ct = contentTypeFor(f.Name)
	}
	h.Set("Content-Type", ct)
	return h
}

// withAttachments re-marshals a request body with the attachments array added.
func withAttachments(body any, files []File) ([]byte, error) {
	metas := make([]attachmentMeta, len(files))
	for i, f := range files {
		metas[i] = attachmentMeta{ID: i, Filename: f.Name, Description: f.Description}
	}

	// Round-trip through a map so the attachments key can be added to any
	// body shape without every payload type needing an Attachments field.
	fields := map[string]any{}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("starlings: encoding request body: %w", err)
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("starlings: encoding request body: %w", err)
		}
	}
	// Initial interaction callbacks wrap message fields in "data", while
	// messages, webhooks and follow-ups carry attachments at the top level.
	_, response := body.(InteractionResponse)
	_, responsePointer := body.(*InteractionResponse)
	if response || responsePointer {
		data, _ := fields["data"].(map[string]any)
		if data == nil {
			data = make(map[string]any)
			fields["data"] = data
		}
		data["attachments"] = metas
	} else {
		fields["attachments"] = metas
	}

	return json.Marshal(fields)
}

// contentTypeFor guesses a MIME type from a filename. Discord only needs it to
// be plausible; it sniffs the bytes itself.
func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".json":
		return "application/json"
	case ".txt", ".log", ".md":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// escapeQuotes protects a filename that contains quotes or backslashes from
// breaking the Content-Disposition header.
func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// SendFiles posts a message with attachments.
//
// Files are limited by the guild's boost tier - 10 MiB by default. Pass an
// empty content to send attachments alone.
func (c *Client) SendFiles(ctx context.Context, channelID Snowflake, data SendData, files ...File) (*Message, error) {
	var msg Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/messages",
		Route:  "POST /channels/" + channelID.String() + "/messages",
		Body:   data,
		Files:  files,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ReplyFiles answers a message with attachments.
func (m *MessageCreate) ReplyFiles(data SendData, files ...File) (*Message, error) {
	if data.MessageRef == nil {
		data.MessageRef = &MessageRef{MessageID: m.ID, ChannelID: m.ChannelID}
	}
	return m.c.SendFiles(context.Background(), m.ChannelID, data, files...)
}

// ExecuteWebhookFiles posts a webhook message with attachments.
func (c *Client) ExecuteWebhookFiles(ctx context.Context, webhookID Snowflake, token string, msg WebhookMessage, wait bool, threadID Snowflake, files ...File) (*Message, error) {
	v := url.Values{}
	if wait {
		v.Set("wait", "true")
	}
	if !threadID.IsZero() {
		v.Set("thread_id", threadID.String())
	}
	req := request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) + query(v),
		Route:  "POST /webhooks/" + webhookID.String() + "/{token}", Body: msg, Files: files,
	}
	if !wait {
		return nil, c.rest.do(ctx, req, nil)
	}
	var out Message
	if err := c.rest.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditWebhookMessageFiles edits a webhook message and uploads attachments.
func (c *Client) EditWebhookMessageFiles(ctx context.Context, webhookID Snowflake, token string, messageID Snowflake, msg WebhookMessage, files ...File) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path: "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) +
			"/messages/" + messageID.String(),
		Route: "PATCH /webhooks/" + webhookID.String() + "/{token}/messages/{id}",
		Body:  msg, Files: files,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
