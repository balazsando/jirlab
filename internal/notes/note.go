// Package notes provides the Note data model and filesystem-backed persistence.
//
// File format (properties header + body):
//
//	id=<id>
//	title=<title>
//	category=<category>
//	priority=<0|1|2>
//	created=<unix-nano>
//	updated=<unix-nano>
//	task=<taskID>:<done>:<text>   (one line per task; text may contain colons)
//	---
//	<description — may span multiple lines>
package notes

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Priority represents a note's urgency level.
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityMedium Priority = 1
	PriorityHigh   Priority = 2
)

func (p Priority) String() string {
	switch p {
	case PriorityHigh:
		return "High"
	case PriorityMedium:
		return "Med"
	default:
		return "Low"
	}
}

// Task is a checklist item inside a Note.
type Task struct {
	ID   string
	Text string
	Done bool
}

// Note is the core data model.
type Note struct {
	ID          string
	Title       string
	Category    string
	Priority    Priority
	Description string
	Tasks       []Task
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const bodySeparator = "---"

// Encode serialises n into the compact properties+body format.
// The result is UTF-8 plain text with no padding or compression.
func Encode(n Note) []byte {
	var b bytes.Buffer

	writeln := func(k, v string) {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}

	writeln("id", n.ID)
	writeln("title", n.Title)
	writeln("category", n.Category)
	writeln("priority", strconv.Itoa(int(n.Priority)))
	writeln("created", strconv.FormatInt(n.CreatedAt.UnixNano(), 10))
	writeln("updated", strconv.FormatInt(n.UpdatedAt.UnixNano(), 10))

	for _, t := range n.Tasks {
		done := "false"
		if t.Done {
			done = "true"
		}
		// format: task=<id>:<done>:<text>
		// text may contain colons; split on the first two colons only when decoding.
		fmt.Fprintf(&b, "task=%s:%s:%s\n", t.ID, done, t.Text)
	}

	b.WriteString(bodySeparator + "\n")
	b.WriteString(n.Description)

	return b.Bytes()
}

// Decode parses data produced by Encode into a Note.
func Decode(data []byte) (Note, error) {
	text := string(data)
	sepIdx := strings.Index(text, "\n"+bodySeparator+"\n")
	var header, body string
	if sepIdx == -1 {
		// tolerate files without a trailing body section
		header = text
	} else {
		header = text[:sepIdx]
		body = text[sepIdx+len("\n"+bodySeparator+"\n"):]
	}

	var n Note
	n.Description = body

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimRight(line, "\r") // handle CRLF
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
			n.ID = val
		case "title":
			n.Title = val
		case "category":
			n.Category = val
		case "priority":
			p, err := strconv.Atoi(val)
			if err != nil {
				return Note{}, fmt.Errorf("decode priority: %w", err)
			}
			n.Priority = Priority(p)
		case "created":
			ns, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return Note{}, fmt.Errorf("decode created: %w", err)
			}
			n.CreatedAt = time.Unix(0, ns)
		case "updated":
			ns, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return Note{}, fmt.Errorf("decode updated: %w", err)
			}
			n.UpdatedAt = time.Unix(0, ns)
		case "task":
			t, err := decodeTask(val)
			if err != nil {
				return Note{}, fmt.Errorf("decode task: %w", err)
			}
			n.Tasks = append(n.Tasks, t)
		}
	}

	if n.ID == "" {
		return Note{}, fmt.Errorf("decode: missing id")
	}
	return n, nil
}

// decodeTask parses "<id>:<done>:<text>" where text may contain colons.
func decodeTask(val string) (Task, error) {
	first := strings.IndexByte(val, ':')
	if first < 0 {
		return Task{}, fmt.Errorf("invalid task format: %q", val)
	}
	id := val[:first]
	rest := val[first+1:]

	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return Task{}, fmt.Errorf("invalid task format: %q", val)
	}
	doneStr := rest[:second]
	text := rest[second+1:]

	done := doneStr == "true"
	return Task{ID: id, Done: done, Text: text}, nil
}
