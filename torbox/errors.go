package torbox

import "fmt"

func FormatAPIError(resp *APIResponse) error {
	if resp == nil {
		return fmt.Errorf("unknown error: empty response")
	}
	if resp.Success {
		return nil
	}

	errorType := resp.Error
	detail := resp.Detail

	// Fallbacks if one is empty
	if detail == "" && errorType != "" {
		detail = errorType
	}
	if errorType == "" && detail != "" {
		errorType = "API_ERROR"
	}
	if errorType == "" && detail == "" {
		return fmt.Errorf("unknown API error occurred")
	}

	// Map known errors to user-friendly messages
	switch errorType {
	case "UNSUPPORTED_SITE":
		return fmt.Errorf("Unsupported site: %s", detail)
	case "TEMPORARILY_DISABLED":
		return fmt.Errorf("Temporarily disabled: %s", detail)
	case "INVALID_OPTION":
		return fmt.Errorf("Invalid option: %s", detail)
	case "DOWNLOAD_SERVER_ERROR":
		return fmt.Errorf("Torbox download server error: %s", detail)
	case "AUTH_ERROR":
		return fmt.Errorf("Authentication error with Torbox: %s", detail)
	case "INVALID_LINK":
		return fmt.Errorf("Invalid link provided: %s", detail)
	case "BAD_TOKEN":
		return fmt.Errorf("Bad Torbox API token: %s", detail)
	case "ACTIVE_LIMIT":
		return fmt.Errorf("Torbox active downloads limit reached: %s", detail)
	default:
		return fmt.Errorf("%s: %s", errorType, detail)
	}
}
