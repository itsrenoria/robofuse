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

func (e *HTTPError) Error() string {
	return e.Message
}

var HosterUnavailableError = &HTTPError{
	StatusCode: 503,
	Message:    "Hoster is unavailable",
	Code:       "hoster_unavailable",
}

var TrafficExceededError = &HTTPError{
	StatusCode: 503,
	Message:    "Traffic exceeded",
	Code:       "traffic_exceeded",
}

var ErrLinkBroken = &HTTPError{
	StatusCode: 404,
	Message:    "File is unavailable",
	Code:       "file_unavailable",
}

var TorrentNotFoundError = &HTTPError{
	StatusCode: 404,
	Message:    "Torrent not found",
	Code:       "torrent_not_found",
}

var NeedsRepairError = &HTTPError{
	StatusCode: 503,
	Message:    "Torrent needs repair",
	Code:       "needs_repair",
}
