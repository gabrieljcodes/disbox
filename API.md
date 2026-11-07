# Disbox Public REST API

Disbox provides a public REST API that allows you to integrate your downloads with other services (like Sonarr, Radarr, or custom scripts) and manage the bot programmatically.

## Authentication

API endpoints are authenticated via Bearer tokens. To generate a token:
1. Log in to the Disbox Web Dashboard
2. Go to the **API Keys** tab
3. Create a new token and copy it

Include the token in your requests using the `Authorization` header:
```bash
Authorization: Bearer dbx_your_token_here
```

## Endpoints

All responses follow a consistent JSON format:
```json
{
  "success": true,
  "data": { ... }
}
```

If an error occurs, the response will look like:
```json
{
  "success": false,
  "error": "Error message here"
}
```

---

### User & Profile Endpoints

#### Get User Details
`GET /v1/me`
Retrieves basic information about the token owner (username, ID, etc).

#### Get User Profile Statistics
`GET /v1/user/profile`
Retrieves user statistics, such as total downloaded bytes, monthly downloaded bytes, and checking if FTP is configured.

---

### FTP Configuration Endpoints

#### Update FTP Settings
`POST /v1/user/ftp`
Body:
```json
{
  "host": "ftp.example.com",
  "username": "my_user",
  "password": "my_password"
}
```
Configures the user's FTP server details. These details are securely stored.

#### Send Download to FTP
`POST /v1/ftp/send`
Body: `{"token": "download_token_here"}`
Triggers a background process that will download the specified file from TorBox and stream it directly to your configured FTP server.

---

### Downloads Endpoints

#### Add Torrent
`POST /v1/add-torrent`
Body: `{"link": "magnet:?xt=urn:btih:..."}`
Adds a torrent and returns a proxy download link.

#### Add Torrent File
`POST /v1/add-torrent-file`
Body: `multipart/form-data` with a `file` field containing the `.torrent` file.
Adds a torrent from a `.torrent` file and returns a proxy download link.

#### Add Web Download
`POST /v1/add-webdl`
Body: `{"link": "https://hoster.com/file..."}`
Adds a direct download link.

#### Remove Download
`POST /v1/remove-download`
Body: `{"token": "download_token_here"}` or `{"tokens": ["token1", "token2"]}`
Removes one or more downloads from the Disbox proxy and deletes them from TorBox as well.

#### Remove Downloads (Mass Delete)
`POST /v1/remove-downloads`
Body:
```json
{
  "tokens": ["token_1", "token_2", "token_3"]
}
```
Deletes multiple downloads in a single batch request from Disbox and TorBox. Returns details on successfully removed and failed items:
```json
{
  "success": true,
  "data": {
    "deleted": ["token_1", "token_2"],
    "failed": [],
    "count": 2,
    "message": "2 download(s) removed"
  }
}
```

#### Get History
`GET /v1/history`
Returns the user's download history and active proxy links.

#### Get Queue Status
`GET /v1/queue-status`
Returns the global slots capacity, active jobs, and queued jobs. This helps identify if a new download will be queued or start immediately.

#### Get Queue Items
`GET /v1/queue`
Returns the user's queued downloads. 
*Admin Note:* Administrators can optionally append `?user_id=123456789` to view a specific user's queue, or omit the query parameter to view the entire global queue.

#### Remove Queue Item
`DELETE /v1/queue/{id}`
Removes an item from the queue by its ID. Admin users can remove any user's item.

#### Move Queue Item
`PATCH /v1/queue/{id}/position`
Body: `{"new_position": 1}`
Moves an item to a new 0-indexed position in the queue. Admin users can move any user's item.

#### Get Hoster List
`GET /v1/hosters`
Returns a dynamic list of hosters that TorBox supports. 
The system automatically aggregates limits across all available API keys. For example, if you have two keys each with a 50GB limit for a hoster, the API will display an aggregated limit of 100GB.

---

### Search Endpoints

*Note: Search endpoints may return HTTP 403 if the administrator has disabled the Search Torrents feature.*

#### Search TMDB
`GET /v1/tmdb/search?query=Matrix&type=movie`
Searches TMDB for movies or series. Returns standard metadata. 

#### Search AniList (Anime)
`GET /v1/anilist/search?query=Naruto`
Searches AniList for anime by title. Returns standard metadata. 

#### Search AIOStreams
`GET /v1/search?query=tmdb:27205&type=movie`
Searches for torrents on AIOStreams. For series, the query format must include season and episode (e.g. `tmdb:2131:1:3`). TMDB IDs are automatically resolved to IMDB IDs in the background.

---

### Admin Endpoints (Access Control)

*Note: These endpoints require the API token to belong to a user listed in `ADMIN_USERS` in the `.env` file.*

#### Get Global Announcements
`GET /v1/announcements`
Returns an array of active global announcements. This endpoint is public for all authenticated users.

#### **Add Global Announcement**
Adds a new announcement banner for all users.

* **URL:** `/v1/admin/announcements/add`
* **Method:** `POST`
* **Auth Required:** Yes (Admin only)
* **Data Params:**
  ```json
  {
      "message": "String"
  }
  ```
* **Success Response:**
  * **Code:** 200 OK

#### **Remove Global Announcement**
Removes a specific announcement.

* **URL:** `/v1/admin/announcements/remove`
* **Method:** `POST`
* **Auth Required:** Yes (Admin only)
* **Data Params:**
  ```json
  {
      "id": "String"
  }
  ```
* **Success Response:**
  * **Code:** 200 OK

#### **Clear All Announcements**
Removes all active announcements.

* **URL:** `/v1/admin/announcements/clear`
* **Method:** `POST`
* **Auth Required:** Yes (Admin only)
* **Success Response:**
  * **Code:** 200 OKUsers
`GET /v1/admin/access`
Returns the current whitelist/blacklist status and a list of all users in the access control list.

#### Check Specific User
`GET /v1/admin/access/check?user_id=123456789`
Returns the access status (`whitelist`, `blacklist`, or `none`) for a specific Discord ID.

#### Add User to Access List
`POST /v1/admin/access/add`
Body: 
```json
{
  "user_id": "123456789",
  "type": "whitelist" // or "blacklist"
}
```
Adds a user to the specified access list.

#### Remove User from Access List
`POST /v1/admin/access/remove`
Body: 
```json
{
  "user_id": "123456789"
}
```
Removes a user from the access list.

#### Toggle Access List Status
`POST /v1/admin/access/toggle`
Body: 
```json
{
  "list_type": "whitelist", // or "blacklist"
  "enabled": true // or false
}
```
Enables or disables the whitelist or blacklist mode globally. (Note: Enabling one will automatically disable the other).

---

#### Magnet To File
`POST /v1/torrents/magnettofile`
Converts any magnet to a torrent file object. Returns TorBox APIResponse structure. Free endpoint (does not consume API limits).
Body:
```json
{
  "magnet": "magnet:?xt=urn:btih:..."
}
```

#### Export Torrent Data
`GET /v1/torrents/exportdata?token=&type=`
Exports the magnet or torrent file.
**Query Parameters**:
- `token`: The download token from the history endpoint.
- `type`: Either "magnet" or "file".

If `type=magnet`, it returns a JSON response with the magnet link in the `data` key. If `type=file`, it responds with a `.torrent` file download.

---

---

## Model Context Protocol (MCP)

Disbox includes a native **Model Context Protocol (MCP)** server, allowing AI agents (Claude Desktop, Cursor, Antigravity, Open WebUI, custom bots) to interact with Disbox directly through structured tool calling.

### Connecting to Disbox via MCP

#### 1. Server-Sent Events (SSE) / Remote Agents
When Disbox is running, the MCP SSE server is available at:
```
http://<disbox-host>:<port>/v1/mcp/sse
```
Authenticate by supplying your Bearer token in the request headers:
```json
{
  "mcpServers": {
    "disbox": {
      "url": "http://localhost:8080/v1/mcp/sse",
      "headers": {
        "Authorization": "Bearer dbx_your_token_here"
      }
    }
  }
}
```
Or via query parameter: `http://localhost:8080/v1/mcp/sse?token=dbx_your_token_here`.

#### 2. Local CLI Stdio Mode
Run the Disbox binary with the `--mcp` flag in your MCP client configuration:
```json
{
  "mcpServers": {
    "disbox": {
      "command": "/path/to/disbox",
      "args": ["--mcp"],
      "env": {
        "DISBOX_API_KEY": "dbx_your_token_here"
      }
    }
  }
}
```

### Exposed MCP Tools

| Tool | Parameters | Description |
| :--- | :--- | :--- |
| `disbox_search_media` | `query` (str, req), `type` (movie/series/anime) | Search torrents/streams across Cinemeta, TMDB, and AIOStreams indexers |
| `disbox_search_tmdb` | `query` (str, req), `type` (movie/series) | Search The Movie Database metadata catalog for titles, summaries, and release dates |
| `disbox_search_anime` | `query` (str, req) | Search AniList GraphQL catalog for anime series and metadata |
| `disbox_search_games` | `query` (str, req) | Search video game catalog via IGDB and indexed sources |
| `disbox_get_game_downloads` | `title` (str, req) | Search for available game repacks and torrent magnets across Hydra sources and scrapers |
| `disbox_add_torrent` | `link` (str, req) | Queue a magnet link or .torrent URL in Disbox/TorBox |
| `disbox_add_webdl` | `link` (str, req) | Queue a direct hoster/debrid download link in Disbox/TorBox |
| `disbox_get_history` | `limit` (int, opt) | List completed and active downloads with direct proxy download URLs and file sizes |
| `disbox_get_queue` | *none* | Check the current queue status, capacity, and active download jobs |
| `disbox_send_ftp` | `token` (str, req), `file_id` (int, opt) | Stream a completed download directly to your configured FTP server/NAS |

---

## Example Request

```bash
curl -X POST http://localhost:8080/v1/add-torrent \
  -H "Authorization: Bearer dbx_123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{"link": "magnet:?xt=urn:btih:example"}'
```
