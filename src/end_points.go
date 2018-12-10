package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	libDatabox "github.com/me-box/lib-go-databox"
	"github.com/zmb3/spotify"
)

var lastUsedRedirectURI = ""

func completeAuth(w http.ResponseWriter, r *http.Request) {
	libDatabox.Info("Callback handle")

	auth := newSpotifyAuthenticator(lastUsedRedirectURI)
	tok, err := auth.Token(state, r)
	if err != nil {
		http.Error(w, "Could not get token", http.StatusForbidden)
		fmt.Println("Error ", err)
		return
	}
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		fmt.Println("State mismatch: " + st + " != " + state + " \n")
		return
	}

	libDatabox.Info("Referer:" + r.Referer())

	fmt.Fprintf(w, "<html><head><script>window.parent.location = '%s';</script><head><body><body></html>", PostAuthCallbackUrl)

	//reset the PostAuthCallbackUrl in case we need to auth again
	PostAuthCallbackUrl = DefaultPostAuthCallbackUrl

	client := auth.NewClient(tok)

	if !DoDriverWorkRunning {
		stopChan = make(chan struct{})
		updateChan = make(chan int)
		go driverWork(client, stopChan, updateChan)
	} else {
		updateChan <- 1
	}

	//save the AccessToken so we can use it if the driver is restarted
	tocJson, _ := json.Marshal(tok)
	storeClient.KVText.Write("auth", "AccessToken", tocJson)
}

func info(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "<h1>Authenticated</h1>")
	fmt.Fprintf(w, "<p>Driver logged in and getting data</p>")
	fmt.Fprintf(w, `<div style="float:right"><a href="/spotify-history-driver/ui/logout">logout</a></div>`)
	artistKeys, err := storeClient.KVJSON.ListKeys("SpotifyTopArtists")
	if err != nil {
		libDatabox.Err("<p>Error could not read artists list " + err.Error() + "</p>")
		return
	}
	genreKeys, err := storeClient.KVJSON.ListKeys("SpotifyTopGenres")
	if err != nil {
		libDatabox.Err("<p>Error could not read artists list " + err.Error() + "</p>")
		return
	}

	fmt.Fprint(w, "<h2 style='clear:both'>Top artists</h2>")
	fmt.Fprint(w, "<pre>")
	for _, key := range artistKeys {
		artist, _ := storeClient.KVJSON.Read("SpotifyTopArtists", key)
		fmt.Fprint(w, string(artist)+"\n")
	}
	fmt.Fprint(w, "</pre>")

	fmt.Fprint(w, "<h2 style='clear:both'>Top Genres</h2>")
	fmt.Fprint(w, "<pre style='clear:both'>")
	for _, key := range genreKeys {
		genre, _ := storeClient.KVJSON.Read("SpotifyTopGenres", key)
		fmt.Fprint(w, string(genre)+"\n")
	}
	fmt.Fprint(w, "</pre>")

	fmt.Fprint(w, "<h2 style='clear:both'>Top Tracks</h2>")
	fmt.Fprint(w, `<div style="width:100%">`)
	trackData, _ := storeClient.KVJSON.Read("SpotifyTrackData", "tracks")
	var tracks []spotify.FullTrack
	json.Unmarshal(trackData, &tracks)
	for _, t := range tracks {
		fmt.Fprint(w, `<img style="display:block;width:20%;margin:1%;float:left" src="`+t.Album.Images[0].URL+`"/>`)
	}
	fmt.Fprint(w, "</div>")

	fmt.Fprint(w, "<pre style='clear:both'>")
	fmt.Fprint(w, string(trackData)+"\n")
	fmt.Fprint(w, "</pre>")

}

func logOut(w http.ResponseWriter, r *http.Request) {
	err := storeClient.KVText.Delete("auth", "AccessToken")
	libDatabox.ChkErr(err)
	artistKeys, err := storeClient.KVJSON.ListKeys("SpotifyTopArtists")
	libDatabox.ChkErr(err)
	for _, key := range artistKeys {
		storeClient.KVJSON.Delete("SpotifyTopArtists", key)
		libDatabox.ChkErr(err)
	}
	genreKeys, err := storeClient.KVJSON.ListKeys("SpotifyTopGenres")
	libDatabox.ChkErr(err)
	for _, key := range genreKeys {
		storeClient.KVJSON.Delete("SpotifyTopGenres", key)
		libDatabox.ChkErr(err)
	}
	aerr := storeClient.KVJSON.Delete("SpotifyTrackData", "tracks")
	libDatabox.ChkErr(aerr)

	go func() {
		close(stopChan)
	}()
	http.Redirect(w, r, "/ui", 302)
}

func authHandle(w http.ResponseWriter, r *http.Request) {

	callbackUrl := r.FormValue("post_auth_callback")
	if DataboxTestMode {
		PostAuthCallbackUrl = "/ui/info"
	}
	if callbackUrl != "" {
		PostAuthCallbackUrl = callbackUrl
	}

	accToken, err := storeClient.KVText.Read("auth", "AccessToken")
	libDatabox.ChkErr(err)
	if len(accToken) > 0 {
		//we are logged in
		if callbackUrl != "" {
			fmt.Fprintf(w, "<html><head><script>window.parent.location = '%s';</script><head><body><body></html>", PostAuthCallbackUrl)
			updateChan <- 1
		} else {
			http.Redirect(w, r, "/ui/info", 302)
		}
		return
	}

	//add the extract the hostname for databox from the passed value
	uri := r.FormValue("databox_uri")
	lastUsedRedirectURI = uri + RedirectURI

	auth := newSpotifyAuthenticator(lastUsedRedirectURI)
	url := auth.AuthURL(state)
	libDatabox.Info("Auth handle")
	fmt.Fprintf(w, "<html><head><script>window.parent.postMessage({ type:'databox_oauth_redirect', url: '%s'}, '*');</script><head><body><body></html>", url)
}

func startAuth(w http.ResponseWriter, r *http.Request) {
	//Display authentication page
	accToken, err := storeClient.KVText.Read("auth", "AccessToken")
	libDatabox.ChkErr(err)
	if len(accToken) > 0 {
		//we are logged in 302 to the info page
		http.Redirect(w, r, "/ui/info", 302)
		return
	}

	fmt.Fprintf(w, "<h1>Authenticate</h1>")
	fmt.Fprintf(w, "<title>Authentication Page</title>")

	fmt.Fprintf(w, `<a href='#' onclick="window.location = './ui/auth?databox_uri=' + window.location.href.split('/').slice(0, 3).join('/')">Press to authenticate</a><br/>`)

}
