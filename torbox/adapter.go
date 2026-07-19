package torbox

// DownloadAdapter provides a unified interface for torrent and web download operations.
// Two adapters justify the seam: TorrentAdapter and WebDLAdapter — both real.
type DownloadAdapter interface {
	GetInfo(id int) (*DownloadInfo, error)
	RequestURL(id int, fileID int, userIP string) (string, error)
	Control(id int, operation string, all bool) (*APIResponse, error)
}

// DownloadInfo is a unified representation of download metadata.
type DownloadInfo struct {
	ID               int
	Hash             string
	Name             string
	Size             int64
	Progress         float64
	DownloadSpeed    int64
	DownloadState    string
	Downloaded       int64
	DownloadPresent  bool
	DownloadFinished bool
	Active           bool
	Files            []TorrentFile
}

// TorrentAdapter adapts the torrent-specific client methods to the DownloadAdapter interface.
type TorrentAdapter struct {
	Client *Client
}

func (a *TorrentAdapter) GetInfo(id int) (*DownloadInfo, error) {
	info, err := a.Client.GetTorrentInfo(id)
	if err != nil {
		return nil, err
	}
	return &DownloadInfo{
		ID:               info.ID,
		Hash:             info.Hash,
		Name:             info.Name,
		Size:             info.Size,
		Progress:         info.Progress,
		DownloadSpeed:    info.DownloadSpeed,
		DownloadState:    info.DownloadState,
		Downloaded:       info.Downloaded,
		DownloadPresent:  info.DownloadPresent,
		DownloadFinished: info.DownloadFinished,
		Active:           info.Active,
		Files:            info.Files,
	}, nil
}

func (a *TorrentAdapter) RequestURL(id int, fileID int, userIP string) (string, error) {
	return a.Client.RequestDownloadURL(id, fileID, userIP)
}

func (a *TorrentAdapter) Control(id int, operation string, all bool) (*APIResponse, error) {
	return a.Client.ControlTorrent(id, operation, all)
}

// WebDLAdapter adapts the web download-specific client methods to the DownloadAdapter interface.
type WebDLAdapter struct {
	Client *Client
}

func (a *WebDLAdapter) GetInfo(id int) (*DownloadInfo, error) {
	info, err := a.Client.GetWebDownloadInfo(id)
	if err != nil {
		return nil, err
	}
	return &DownloadInfo{
		ID:               info.ID,
		Name:             info.Name,
		Size:             info.Size,
		Progress:         info.Progress,
		DownloadSpeed:    info.DownloadSpeed,
		DownloadState:    info.DownloadState,
		Downloaded:       info.Downloaded,
		DownloadPresent:  info.DownloadPresent,
		DownloadFinished: info.DownloadFinished,
		Active:           info.Active,
		Files:            info.Files,
	}, nil
}

func (a *WebDLAdapter) RequestURL(id int, fileID int, userIP string) (string, error) {
	return a.Client.RequestWebDownloadURL(id, fileID, userIP)
}

func (a *WebDLAdapter) Control(id int, operation string, all bool) (*APIResponse, error) {
	return a.Client.ControlWebDownload(id, operation, all)
}
