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

### User Endpoints

#### Get User Profile
`GET /v1/me`
Retrieves information about the token owner.

#### Add Torrent
`POST /v1/add-torrent`
Body: `{"link": "magnet:?xt=urn:btih:..."}`
Adds a torrent and returns a proxy download link.

#### Add Web Download
`POST /v1/add-webdl`
Body: `{"link": "https://hoster.com/file..."}`
Adds a direct download link.

#### Remove Download
`POST /v1/remove-download`
Body: `{"token": "download_token_here"}`
Removes a download from the Disbox proxy and deletes it from TorBox.

#### Get History
`GET /v1/history`
Returns the user's download history and active proxy links.

---

### Admin Endpoints (Access Control)

*Note: These endpoints require the API token to belong to a user listed in `ADMIN_USERS` in the `.env` file.*

#### List Access Control Users
`GET /v1/admin/access`
Returns the current whitelist/blacklist status and a list of all users in the access control list.

#### Check Specific User
`GET /v1/admin/access/check?discord_id=123456789`
Returns the access status (`whitelist`, `blacklist`, or `none`) for a specific Discord ID.

#### Add User to Access List
`POST /v1/admin/access/add`
Body: 
```json
{
  "discord_id": "123456789",
  "type": "whitelist" // or "blacklist"
}
```
Adds a user to the specified access list.

#### Remove User from Access List
`POST /v1/admin/access/remove`
Body: 
```json
{
  "discord_id": "123456789"
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

## Example Request

```bash
curl -X POST http://localhost:8080/v1/add-torrent \
  -H "Authorization: Bearer dbx_123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{"link": "magnet:?xt=urn:btih:example"}'
```
