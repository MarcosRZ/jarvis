package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Operation kinds that the dashboard queues for the client.
const (
	OperationUpsert = "upsert"
	OperationDelete = "delete"
)

// ErrInvalidOperation marks a queued operation that the client must drop
// instead of retrying. A write error is not wrapped with it, so the client can
// apply the operation again on the next poll.
var ErrInvalidOperation = errors.New("invalid learning operation")

// Operation is one change queued from the dashboard. IDs increase over time;
// the client acknowledges every operation it has handled in its next snapshot.
type Operation struct {
	ID           int64     `json:"id"`
	Kind         string    `json:"kind"`
	OriginalText string    `json:"originalText,omitempty"`
	Entry        *Entry    `json:"entry,omitempty"`
	Texts        []string  `json:"texts,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type operationsResponse struct {
	Operations []Operation `json:"operations"`
}

// OperationsURL returns the operations endpoint that belongs to a dashboard
// snapshot URL.
func OperationsURL(snapshotURL string) string {
	base := strings.TrimRight(strings.TrimSpace(snapshotURL), "/")
	if strings.HasSuffix(base, "/operations") {
		return base
	}
	return base + "/operations"
}

// FetchOperations asks the dashboard for the operations after id.
func FetchOperations(ctx context.Context, client *http.Client, endpoint string, after int64) ([]Operation, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse learning operations URL: %w", err)
	}
	query := u.Query()
	query.Set("after", strconv.FormatInt(after, 10))
	u.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create learning operations request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch learning operations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		detail := strings.TrimSpace(string(message))
		if detail == "" {
			return nil, fmt.Errorf("fetch learning operations: %s", response.Status)
		}
		return nil, fmt.Errorf("fetch learning operations: %s: %s", response.Status, detail)
	}
	var body operationsResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode learning operations: %w", err)
	}
	return body.Operations, nil
}

// ApplyOperation applies one queued dashboard operation. It returns whether
// the store changed. A malformed operation returns ErrInvalidOperation and is
// safe to acknowledge; a write failure is returned without that marker and
// should be retried.
func (s *Store) ApplyOperation(op Operation, now time.Time) (bool, error) {
	switch op.Kind {
	case OperationUpsert:
		if op.Entry == nil {
			return false, fmt.Errorf("%w: upsert without an entry", ErrInvalidOperation)
		}
		if _, err := s.Upsert(op.OriginalText, *op.Entry, now); err != nil {
			if errors.Is(err, ErrInvalidEntry) {
				return false, fmt.Errorf("%w: %v", ErrInvalidOperation, err)
			}
			return false, err
		}
		return true, nil
	case OperationDelete:
		if len(op.Texts) == 0 {
			return false, fmt.Errorf("%w: delete without texts", ErrInvalidOperation)
		}
		removed, err := s.Delete(op.Texts)
		if err != nil {
			return false, err
		}
		return removed > 0, nil
	default:
		return false, fmt.Errorf("%w: unknown kind %q", ErrInvalidOperation, op.Kind)
	}
}
