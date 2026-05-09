package request

// errors.go defines typed HTTP errors returned by request operations.

// HTTPError represents an HTTP error with status code and message
type HTTPError struct {
	StatusCode   int    `json:"status_code"`
	Message      string `json:"message"`
	Code         string `json:"code"`
	RDErrorCode  int    `json:"rd_error_code,omitempty"`  // Real-Debrid error_code from response body
	RDError      string `json:"rd_error,omitempty"`        // Real-Debrid error message from response body
}

// Error implements the error interface for HTTPError.
func (e *HTTPError) Error() string {
	return e.Message
}

// HosterUnavailableError is returned when the Real-Debrid hoster is temporarily unavailable (503).
var HosterUnavailableError = &HTTPError{
	StatusCode: 503,
	Message:    "Hoster is unavailable",
	Code:       "hoster_unavailable",
}

// TrafficExceededError is returned when the Real-Debrid account has exceeded its traffic limit (503).
var TrafficExceededError = &HTTPError{
	StatusCode: 503,
	Message:    "Traffic exceeded",
	Code:       "traffic_exceeded",
}

// ErrLinkBroken is returned when a file link is no longer available (404).
var ErrLinkBroken = &HTTPError{
	StatusCode: 404,
	Message:    "File is unavailable",
	Code:       "file_unavailable",
}

// TorrentNotFoundError is returned when a torrent cannot be found on Real-Debrid (404).
var TorrentNotFoundError = &HTTPError{
	StatusCode: 404,
	Message:    "Torrent not found",
	Code:       "torrent_not_found",
}

// NeedsRepairError is returned when a torrent needs to be re-added (503).
var NeedsRepairError = &HTTPError{
	StatusCode: 503,
	Message:    "Torrent needs repair",
	Code:       "needs_repair",
}
