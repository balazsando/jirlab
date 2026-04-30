// Package templates provides the Template data model and filesystem-backed persistence.
//
// File format (properties header + body):
//
//	id=<id>
//	name=<name>
//	created=<unix-nano>
//	updated=<unix-nano>
//	---
//	<body — may span multiple lines>
package templates

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Template is a reusable Jira ticket comment template.
type Template struct {
	ID        string
	Name      string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const bodySeparator = "---"

// Encode serialises t into the compact properties+body format.
func Encode(t Template) []byte {
	var b bytes.Buffer

	writeln := func(k, v string) {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}

	writeln("id", t.ID)
	writeln("name", t.Name)
	writeln("created", strconv.FormatInt(t.CreatedAt.UnixNano(), 10))
	writeln("updated", strconv.FormatInt(t.UpdatedAt.UnixNano(), 10))
	b.WriteString(bodySeparator + "\n")
	b.WriteString(t.Body)

	return b.Bytes()
}

// Decode parses data produced by Encode into a Template.
func Decode(data []byte) (Template, error) {
	text := string(data)
	sepIdx := strings.Index(text, "\n"+bodySeparator+"\n")
	var header, body string
	if sepIdx == -1 {
		header = text
	} else {
		header = text[:sepIdx]
		body = text[sepIdx+len("\n"+bodySeparator+"\n"):]
	}

	var t Template
	t.Body = body

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+1:]

		switch key {
		case "id":
			t.ID = val
		case "name":
			t.Name = val
		case "created":
			ns, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return Template{}, fmt.Errorf("decode created: %w", err)
			}
			t.CreatedAt = time.Unix(0, ns)
		case "updated":
			ns, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return Template{}, fmt.Errorf("decode updated: %w", err)
			}
			t.UpdatedAt = time.Unix(0, ns)
		}
	}

	if t.ID == "" {
		return Template{}, fmt.Errorf("decode: missing id")
	}
	return t, nil
}

// NewID generates a collision-resistant 8-character hex ID.
func NewID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
