package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	libDatabox "github.com/me-box/lib-go-databox"
)

func completeAuth(w http.ResponseWriter, r *http.Request) {
	libDatabox.Info("Callback handle")
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

	http.Redirect(w, r, RedirectHostInsideDatabox+"/ui/info", 302)

	client := auth.NewClient(tok)

	channel := make(chan []string)
	stopChan = make(chan int)
	go driverWorkTrack(client, stopChan)
	go driverWorkArtist(client, channel, stopChan)
	go driverWorkGenre(client, channel, stopChan)

	//save the AccessToken so we can use it if the driver is restarted
	tocJson, _ := json.Marshal(tok)
	storeClient.KVText.Write("auth", "AccessToken", tocJson)
}

func info(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "<h1>Authenticated</h1>")
	fmt.Fprintf(w, "<p>Driver logged in and getting data</p>")
	fmt.Fprintf(w, `<div style="float:right"><a href="/spotify-history-driver/ui/logout">logout</a></div>`)
	artistList, err := storeClient.KVText.ListKeys("SpotifyTopArtists")
	if err != nil {
		libDatabox.Err("<p>Error could not read artists list " + err.Error() + "</p>")
		return
	}

	fmt.Fprintf(w, "<h2>Top artists</h2>")
	fmt.Fprintf(w, "<pre>")
	fmt.Fprintf(w, strings.Join(artistList, "\n"))
	fmt.Fprintf(w, "</pre>")

}

func logOut(w http.ResponseWriter, r *http.Request) {
	err := storeClient.KVText.Delete("auth", "AccessToken")
	libDatabox.ChkErr(err)
	go func() {
		stopChan <- 1
		stopChan <- 1
		stopChan <- 1
	}()
	http.Redirect(w, r, RedirectHostInsideDatabox+"/ui", 302)
}

func authHandle(w http.ResponseWriter, r *http.Request) {
	url := auth.AuthURL(state)
	libDatabox.Info("Auth handle")
	fmt.Fprintf(w, "<script>window.parent.postMessage({ type:'databox_oauth_redirect', url: '%s'}, '*');</script>", url)
}

func startAuth(w http.ResponseWriter, r *http.Request) {
	//Display authentication page
	accToken, err := storeClient.KVText.Read("auth", "AccessToken")
	libDatabox.ChkErr(err)
	if len(accToken) > 0 {
		//we are logged in 302 to the info page
		http.Redirect(w, r, RedirectHostInsideDatabox+"/ui/info", 302)
		return
	}

	fmt.Fprintf(w, "<h1>Authenticate</h1>")
	fmt.Fprintf(w, "<title>Authentication Page</title>")

	if DataboxTestMode {
		url := auth.AuthURL(state)
		fmt.Fprintf(w, "<a href='%s'>Press to authenticate</a>", url)
	} else {
		fmt.Fprintf(w, "<a href='./ui/auth'>Press to authenticate</a>")
	}

}
