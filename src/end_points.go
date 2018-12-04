package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	libDatabox "github.com/me-box/lib-go-databox"
)

func completeAuth(w http.ResponseWriter, r *http.Request) {
	libDatabox.Info("Callback handle")

	auth := newSpotifyAuthenticator()
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
		go driverWorkTrack(client, stopChan, updateChan)
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

	fmt.Fprintf(w, "<h2>Top artists</h2>")
	fmt.Fprintf(w, "<pre>")
	for _, key := range artistKeys {
		artist, _ := storeClient.KVJSON.Read("SpotifyTopArtists", key)
		fmt.Fprintf(w, string(artist)+"\n")
	}
	fmt.Fprintf(w, "</pre>")

}

func logOut(w http.ResponseWriter, r *http.Request) {
	err := storeClient.KVText.Delete("auth", "AccessToken")
	libDatabox.ChkErr(err)
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

	//add the extract the hostname for databox from the passed value
	uri := r.FormValue("databox_uri")
	RedirectURI = uri + RedirectURI

	auth := newSpotifyAuthenticator()
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
