# Spotify Random Playlist Generator

A simple Go CLI tool that creates a random Spotify playlist based on a music genre or style.

You run a command like:

```bash
./play heavy metal

The tool searches Spotify for tracks related to the chosen genre, shuffles the results locally, creates a private playlist in your Spotify account, adds the tracks, and opens the playlist automatically in your browser.

Features
Create random Spotify playlists from any genre or style
Automatic Spotify OAuth login
Local token cache using Spotify refresh token
No need to login on every execution
Automatically opens the created playlist in the browser
Configurable track count
Written in Go with no external dependencies
Requirements
Go installed
Spotify account
Spotify Developer app
Linux, macOS, or Windows
Spotify App Setup

Create an app in the Spotify Developer Dashboard:

https://developer.spotify.com/dashboard

Inside your app settings, configure the Redirect URI exactly as:

http://127.0.0.1:8888/callback

Then copy your:

Client ID
Client Secret
Installation

Clone the repository:

git clone https://github.com/YOUR_USERNAME/YOUR_REPOSITORY.git
cd YOUR_REPOSITORY

Initialize Go modules if needed:

go mod init spotify-random-playlist

Make the helper script executable:

chmod +x play
Configuration

Do not commit Spotify credentials to GitHub.

Export them in your shell:

export SPOTIFY_CLIENT_ID="your_client_id"
export SPOTIFY_CLIENT_SECRET="your_client_secret"

Optionally choose which browser should open Spotify:

export BROWSER="opera"

Other examples:

export BROWSER="chromium"
export BROWSER="google-chrome"
export BROWSER="firefox"
Recommended play Script

Create a file named play:

#!/usr/bin/env bash
set -euo pipefail

: "${SPOTIFY_CLIENT_ID:?SPOTIFY_CLIENT_ID is required}"
: "${SPOTIFY_CLIENT_SECRET:?SPOTIFY_CLIENT_SECRET is required}"

export BROWSER="${BROWSER:-opera}"

TRACK_COUNT="${TRACK_COUNT:-50}"

if [ "$#" -eq 0 ]; then
  echo 'Usage: ./play <music genre/style>'
  echo 'Example: ./play heavy metal'
  exit 1
fi

GENRE="$*"

go run main.go "$GENRE" "$TRACK_COUNT"

Then make it executable:

chmod +x play
Usage

Create a playlist with the default amount of tracks:

./play heavy metal

Create a playlist with a custom number of tracks:

TRACK_COUNT=100 ./play death metal

Another example:

TRACK_COUNT=150 ./play progressive rock

The script accepts multiple words as the genre or style:

./play samba rock
./play jazz fusion
./play black metal
./play classic rock
Authentication Flow

On the first execution, the tool opens Spotify login in your browser.

You only need to authorize the app once.

After authorization, the tool saves a local token cache at:

~/.spotify-random-playlist-token.json

On future executions, the tool uses the cached refresh token and does not ask for login again.

To force a new login:

SPOTIFY_FORCE_LOGIN=1 ./play heavy metal

To remove the cached token manually:

rm ~/.spotify-random-playlist-token.json
How It Works

The tool uses the Spotify Web API with this flow:

1. Receives the genre/style from the command line
2. Authenticates with Spotify using OAuth
3. Searches tracks using Spotify Search API
4. Collects multiple pages of results
5. Removes duplicate tracks
6. Shuffles the track list locally
7. Creates a private playlist
8. Adds tracks in batches
9. Opens the playlist in the browser
Project Structure
.
├── main.go    # Main Go application
├── play       # Shell helper script
└── README.md
Environment Variables
Variable	Required	Description
SPOTIFY_CLIENT_ID	Yes	Spotify app client ID
SPOTIFY_CLIENT_SECRET	Yes	Spotify app client secret
TRACK_COUNT	No	Number of tracks to add to the playlist. Defaults to 50
BROWSER	No	Browser command used to open Spotify and the created playlist
SPOTIFY_FORCE_LOGIN	No	Set to 1 to ignore the cached token and login again
Examples
./play heavy metal
TRACK_COUNT=100 ./play death metal
TRACK_COUNT=120 ./play jazz fusion
BROWSER=firefox ./play samba rock
SPOTIFY_FORCE_LOGIN=1 ./play progressive metal
Notes About Spotify API Limits

Spotify limits how many search results can be returned per request.

This project handles that by performing multiple search requests using offsets, collecting results, removing duplicates, and shuffling them locally.

When adding tracks to a playlist, the tool sends them in batches, so playlists with more than 100 tracks are supported.

The final quality of the playlist depends on how well Spotify Search understands the genre or style you provide.

For best results, use specific genre or style terms:

./play melodic death metal
./play brazilian jazz
./play classic hard rock
./play old school thrash metal

Instead of overly broad terms like:

./play rock
./play pop
./play metal
Security

Do not commit your Spotify credentials.

Avoid this in public repositories:

export SPOTIFY_CLIENT_ID="real_client_id"
export SPOTIFY_CLIENT_SECRET="real_client_secret"

Prefer exporting the variables directly in your local shell:

export SPOTIFY_CLIENT_ID="your_client_id"
export SPOTIFY_CLIENT_SECRET="your_client_secret"

Recommended .gitignore:

.env
.env.local
.spotify-random-playlist-token.json
License

MIT License.

Feel free to use, modify, and improve.