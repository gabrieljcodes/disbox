package proxy

import (
	"crypto/sha1"
	"fmt"
	"net/url"
	"strings"
)

// ExtractMagnetFromTorrentFile parses a .torrent file byte buffer, computes the SHA-1 infohash of its
// bencoded info dictionary, and returns a standard magnet URI string (magnet:?xt=urn:btih:HASH&dn=NAME).
func ExtractMagnetFromTorrentFile(fileData []byte) (string, error) {
	if len(fileData) == 0 {
		return "", fmt.Errorf("empty torrent file data")
	}

	infoBytes, name, err := extractInfoDictAndName(fileData)
	if err != nil {
		return "", fmt.Errorf("failed to extract torrent info dictionary: %w", err)
	}

	hash := sha1.Sum(infoBytes)
	hexHash := fmt.Sprintf("%x", hash)

	if name != "" {
		return fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hexHash, url.QueryEscape(name)), nil
	}
	return fmt.Sprintf("magnet:?xt=urn:btih:%s", hexHash), nil
}

// extractInfoDictAndName scans the top-level bencoded dictionary in fileData to find the 'info' key value
// and its 'name' property (if present).
func extractInfoDictAndName(data []byte) ([]byte, string, error) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, "", fmt.Errorf("invalid bencode: must start with dictionary 'd'")
	}

	idx := 1
	var infoBytes []byte
	var name string

	for idx < len(data) && data[idx] != 'e' {
		key, nextIdx, err := parseBencodeString(data, idx)
		if err != nil {
			return nil, "", err
		}
		idx = nextIdx

		if key == "info" {
			valStart := idx
			valEnd, err := skipBencodeValue(data, valStart)
			if err != nil {
				return nil, "", fmt.Errorf("corrupt info dictionary: %w", err)
			}
			infoBytes = data[valStart:valEnd]
			name = extractTorrentNameFromInfoDict(infoBytes)
			return infoBytes, name, nil
		}

		// Skip value for non-info keys
		valEnd, err := skipBencodeValue(data, idx)
		if err != nil {
			return nil, "", err
		}
		idx = valEnd
	}

	return nil, "", fmt.Errorf("'info' dictionary not found in torrent file")
}

func parseBencodeString(data []byte, start int) (string, int, error) {
	colonIndex := -1
	for i := start; i < len(data); i++ {
		if data[i] == ':' {
			colonIndex = i
			break
		}
		if data[i] < '0' || data[i] > '9' {
			return "", start, fmt.Errorf("invalid string length at position %d", i)
		}
	}
	if colonIndex == -1 {
		return "", start, fmt.Errorf("unexpected EOF reading string length")
	}

	var strLen int
	_, err := fmt.Sscanf(string(data[start:colonIndex]), "%d", &strLen)
	if err != nil || strLen < 0 {
		return "", start, fmt.Errorf("invalid string length number")
	}

	strStart := colonIndex + 1
	strEnd := strStart + strLen
	if strEnd > len(data) {
		return "", start, fmt.Errorf("string length exceeds data buffer")
	}

	return string(data[strStart:strEnd]), strEnd, nil
}

func skipBencodeValue(data []byte, start int) (int, error) {
	if start >= len(data) {
		return start, fmt.Errorf("unexpected EOF")
	}

	switch data[start] {
	case 'i':
		// Integer: i<number>e
		for i := start + 1; i < len(data); i++ {
			if data[i] == 'e' {
				return i + 1, nil
			}
		}
		return start, fmt.Errorf("unterminated integer at %d", start)
	case 'l', 'd':
		// List or Dict: l...e / d...e
		curr := start + 1
		for curr < len(data) && data[curr] != 'e' {
			next, err := skipBencodeValue(data, curr)
			if err != nil {
				return start, err
			}
			curr = next
		}
		if curr >= len(data) {
			return start, fmt.Errorf("unterminated list/dict at %d", start)
		}
		return curr + 1, nil
	default:
		// String: <length>:<content>
		if data[start] >= '0' && data[start] <= '9' {
			_, next, err := parseBencodeString(data, start)
			return next, err
		}
		return start, fmt.Errorf("unknown bencode token '%c' at position %d", data[start], start)
	}
}

func extractTorrentNameFromInfoDict(infoDict []byte) string {
	if len(infoDict) == 0 || infoDict[0] != 'd' {
		return ""
	}
	idx := 1
	for idx < len(infoDict) && infoDict[idx] != 'e' {
		key, nextIdx, err := parseBencodeString(infoDict, idx)
		if err != nil {
			break
		}
		idx = nextIdx

		if key == "name" {
			val, _, err := parseBencodeString(infoDict, idx)
			if err == nil {
				return strings.TrimSpace(val)
			}
			break
		}

		valEnd, err := skipBencodeValue(infoDict, idx)
		if err != nil {
			break
		}
		idx = valEnd
	}
	return ""
}
