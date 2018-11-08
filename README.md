# Databox Spotify Driver
## Data format
Data for the Spotify driver is gather using the Spotify API. 
A [Golang wrapper for the Spotify API](https://github.com/zmb3/spotify) is used in in order to access the API calls.

The driver uses Oauth authentication in order to sign users in and allow the driver access to the data.
The Oauth is accessed using a link provided on the drivers UI, which when the user clicks will redirect them to the Spotify authentication page. Then on a success it will redirect back to the UI of the driver.

All data is updated currently every 30 seconds (testing purposes). 
The Spotify API retrives a maximum of 50 entires every call, due to limitations on the API. 

## Data stores
There are three seperate data stores associated with the driver. 

### Track data store
When tracks are downloaded using the Spotify API call, they are stored in a time series blob store **(TSBlob)**, with the time value being to set to when the track was listened too by the user.
The content type that is being store is JSON **(ContentTypeJSON)**
The datastore ID is: ***"SpotifyTrackData"***
### Track data example
```
{"track":
{"artists":[
  {"name":"Fall Out Boy",
  "id":"4UXqAaa6dQYAk18Lv7PEgX","uri":"spotify:artist:4UXqAaa6dQYAk18Lv7PEgX",
  "href":"https://api.spotify.com/v1/artists/4UXqAaa6dQYAk18Lv7PEgX",
  "external_urls":{"spotify":"https://open.spotify.com/artist/4UXqAaa6dQYAk18Lv7PEgX"}}
],
"available_markets":["AD","AR","AT","AU","BE","BG","BO","BR","CA","CH","CL","CO","CR","CY","CZ","DE","DK","DO","EC","EE","ES","FI","FR","GB","GR","GT","HK","HN","HU","ID","IE","IS","IT","JP","LI","LT","LU","LV","MC","MT","MX","MY","NI","NL","NO","NZ","PA","PE","PH","PL","PT","PY","SE","SG","SK","SV","TR","TW","US","UY"],
"disc_number":1,
"duration_ms":243040,
"explicit":false,
"external_urls":{"spotify":"https://open.spotify.com/track/3Q7jFW6nxqhwbUctqqthSa"},
"href":"https://api.spotify.com/v1/tracks/3Q7jFW6nxqhwbUctqqthSa","id":"3Q7jFW6nxqhwbUctqqthSa",
"name":"Where Did The Party Go",
"preview_url":"",
"track_number":4,
"uri":"spotify:track:3Q7jFW6nxqhwbUctqqthSa"},
"played_at":"2018-10-16T15:23:23.394Z",
"context":{"external_urls":{"spotify":"https://open.spotify.com/playlist/701QceRx6bh6MHvZF5LfVh"},
"href":"https://api.spotify.com/v1/playlists/701QceRx6bh6MHvZF5LfVh",
"type":"playlist_v2",
"uri":"spotify:playlist:701QceRx6bh6MHvZF5LfVh"}
}

```
### Artist data store
When the top artists are downloaded by using the Spotify API, they are stored in key-value pairs **(KVStore)**, with the key being their position in the returned object.
The content type that is stored is JSON **(ContentTypeJSON)**
The datastore ID is: ***"SpotifyTopArtists"***
### Artist data example
```
{"name":"Fall Out Boy",
"genres":["emo","modern rock","pop punk"],
"popularity":82,
"id":"4UXqAaa6dQYAk18Lv7PEgX"}

```
### Genre data store
The top genres are calculated from the top artists, as the Spotify API does not have a specific call to retrive the users top genres.
This is done by taking each artists list of genres and ordering them from most common to least common based on how many times they occur in the top artists.
The genres are then store in kev-value pairs **(KVStore)**, with the key being the genres position.
The content type that is store is Text **(ContentTypeText)**
The datastore ID is: ***"SpotifyTopGenres"***
### Genre data example
```
modern rock

```
