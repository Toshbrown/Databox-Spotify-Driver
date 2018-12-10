package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	libDatabox "github.com/me-box/lib-go-databox"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

const (
	RedirectHostInsideDatabox                = ""
	RedirectHostOutsideDatabox               = "http://127.0.0.1:8080"
	OAuthRedirectURIInsideDatabox            = "/core-ui/ui/view/spotify-history-driver/callback"
	OAuthRedirectURIOutsideDatabox           = "/ui/spotify-history-driver/callback"
	testArbiterEndpoint                      = "tcp://127.0.0.1:4444"
	testStoreEndpoint                        = "tcp://127.0.0.1:5555"
	DefaultPostAuthCallbackUrlInsideDatabox  = "/core-ui/ui/view/spotify-history-driver"
	DefaultPostAuthCallbackUrlOutsideDatabox = "/ui/spotify-history-driver"
)

var (
	state                      = ""
	storeClient                *libDatabox.CoreStoreClient
	DataboxTestMode            = os.Getenv("DATABOX_VERSION") == ""
	stopChan                   chan struct{}
	updateChan                 chan int
	PostAuthCallbackUrl        string //where to redirect the user on successful Auth
	DefaultPostAuthCallbackUrl string
	DoDriverWorkRunning        bool
	RedirectURI                string //gets set to the correct redirect URI fro oauth
)

//ArtistArray is an array of artists
type ArtistArray struct {
	Items []Artist
}

//Artist struct contains information based on the artists
type Artist struct {
	Name       string   `json:"name"`
	Genre      []string `json:"genres"`
	Popularity int      `json:"popularity"`
	ID         string   `json:"id"`
	Images     []Image  `json:"images"`
}

// Image identifies an image associated with an item.
type Image struct {
	// The image height, in pixels.
	Height int `json:"height"`
	// The image width, in pixels.
	Width int `json:"width"`
	// The source URL of the image.
	URL string `json:"url"`
}

//Genres contains list of all genres and their occurrences
type Genres struct {
	Name  string
	Count int
}

func main() {
	libDatabox.Info("Starting ....")

	//set up storeClient
	var DataboxStoreEndpoint string
	if DataboxTestMode {
		DataboxStoreEndpoint = testStoreEndpoint
		ac, _ := libDatabox.NewArbiterClient("./", "./", testArbiterEndpoint)
		storeClient = libDatabox.NewCoreStoreClient(ac, "./", DataboxStoreEndpoint, false)
		//turn on debug output for the databox library
		libDatabox.OutputDebug(true)
		PostAuthCallbackUrl = DefaultPostAuthCallbackUrlOutsideDatabox
	} else {
		DataboxStoreEndpoint = os.Getenv("DATABOX_ZMQ_ENDPOINT")
		storeClient = libDatabox.NewDefaultCoreStoreClient(DataboxStoreEndpoint)
		PostAuthCallbackUrl = DefaultPostAuthCallbackUrlInsideDatabox
	}
	DefaultPostAuthCallbackUrl = PostAuthCallbackUrl

	registerDatasources()

	router := mux.NewRouter()
	router.HandleFunc("/status", statusEndpoint).Methods("GET")
	router.HandleFunc("/ui/callback", completeAuth)
	router.HandleFunc("/ui/auth", authHandle)
	router.HandleFunc("/ui/logout", logOut)
	router.HandleFunc("/ui/info", info)
	router.HandleFunc("/ui", startAuth)

	//Do we have an auth token?
	accToken, err := storeClient.KVText.Read("auth", "AccessToken")
	libDatabox.ChkErr(err)
	if len(accToken) > 0 {
		libDatabox.Info("Token found in DB starting driverWorkTrack")
		go func() {
			time.Sleep(time.Second * 10) //give DB some time to set permissions
			var tok *oauth2.Token
			json.Unmarshal(accToken, &tok)

			auth := newSpotifyAuthenticator("https://127.0.0.1")
			client := auth.NewClient(tok)
			stopChan = make(chan struct{})
			updateChan = make(chan int)
			go driverWork(client, stopChan, updateChan)
		}()
	}

	//Set client_id and client_secret for the application inside the auth object
	RedirectURI = OAuthRedirectURIInsideDatabox
	if DataboxTestMode {
		RedirectURI = OAuthRedirectURIOutsideDatabox
	}

	//create a uuid each time we start the driver to use as the
	//state in the oAuth request.
	state = uuid.New().String()

	setUpWebServer(DataboxTestMode, router, "8080")
}

func newSpotifyAuthenticator(oAuthRedirectURI string) *spotify.Authenticator {
	auth := spotify.NewAuthenticator(oAuthRedirectURI,
		spotify.ScopeUserReadPrivate,
		spotify.ScopeUserReadRecentlyPlayed,
		spotify.ScopeUserTopRead)
	auth.SetAuthInfo("2706f5aa27b646d8835a6a8aca7eba37", "eb8aec62450e4d44a4308f07b82338cb")
	return &auth
}

func statusEndpoint(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("active\n"))
}

func registerDatasources() {

	libDatabox.Info("starting driver work")

	trackDatasource := libDatabox.DataSourceMetadata{
		Description:    "Spotify Playlist Data",    //required
		ContentType:    libDatabox.ContentTypeJSON, //required
		Vendor:         "databox-test",             //required
		DataSourceType: "spotify::playlistData",    //required
		DataSourceID:   "SpotifyTrackData",         //required
		StoreType:      libDatabox.StoreTypeKV,     //required
		IsActuator:     false,
		IsFunc:         false,
	}
	tErr := storeClient.RegisterDatasource(trackDatasource)
	if tErr != nil {
		libDatabox.Err("Error Registering Datasource " + tErr.Error())
		return
	}
	libDatabox.Info("Registered Track Datasource")

	artistDatasource := libDatabox.DataSourceMetadata{
		Description:    "Spotify Top 20 user artists", //required
		ContentType:    libDatabox.ContentTypeJSON,    //required
		Vendor:         "databox-test",                //required
		DataSourceType: "spotify::topArtists",         //required
		DataSourceID:   "SpotifyTopArtists",           //required
		StoreType:      libDatabox.StoreTypeKV,        //required
		IsActuator:     false,
		IsFunc:         false,
	}
	aErr := storeClient.RegisterDatasource(artistDatasource)
	if aErr != nil {
		libDatabox.Err("Error Registering Credential Datasource " + aErr.Error())
		return
	}
	libDatabox.Info("Registered Top Artist Datasource")

}

func driverWork(client spotify.Client, stop chan struct{}, forceUpdate chan int) {
	if DoDriverWorkRunning {
		//dont run two of these
		return
	}

	for {
		DoDriverWorkRunning = true
		go driverWorkTrack(client)
		go driverWorkArtist(client)

		select {
		case <-stop:
			libDatabox.Info("Stopping data updates stop message received")
			DoDriverWorkRunning = false
			return
		case <-forceUpdate:
			libDatabox.Info("updating data forced")
		case <-time.After(time.Minute * 30):
			libDatabox.Info("updating data after time out")
		}

	}

}

func driverWorkTrack(client spotify.Client) {

	var opts spotify.Options

	results, err := client.CurrentUsersTopTracksOpt(&opts)
	if err != nil {
		fmt.Println("Error ", err)
		return
	}
	fmt.Println("PlayerRecentlyPlayed", results)
	if len(results.Tracks) > 0 {

		b, err := json.Marshal(results.Tracks)
		if err != nil {
			fmt.Println("Error ", err)
			return
		}
		aerr := storeClient.KVJSON.Write("SpotifyTrackData", "tracks", b)
		if aerr != nil {
			libDatabox.Err("Error Write Datasource " + aerr.Error())
			return
		}

		//Get most recent items time and convert to milliseconds
		/*recentTime = results[0].PlayedAt.Unix() * 1000
		opts.AfterEpochMs = recentTime + 500

		libDatabox.Info("Converting data tracks")
		for i := len(results) - 1; i > -1; i-- {
			b, err := json.Marshal(results[i])
			if err != nil {
				fmt.Println("Error ", err)
				return
			}
			aerr := storeClient.TSBlobJSON.WriteAt("SpotifyTrackData",
				results[i].PlayedAt.Unix()*1000,
				b)
			if aerr != nil {
				libDatabox.Err("Error Write Datasource " + aerr.Error())
			}
		}*/
		libDatabox.Info("Storing data")
	} else {
		libDatabox.Info("No new data")
	}

}

func driverWorkArtist(client spotify.Client) {
	var artists ArtistArray
	fmt.Println("getting CurrentUsersTopArtists")
	results, err := client.CurrentUsersTopArtists()
	if err != nil {
		fmt.Println("Error ", err)
		return
	}
	fmt.Println("CurrentUsersTopArtists", results)
	if results != nil {
		libDatabox.Info("Converting data artists")

		b, bErr := json.Marshal(results)
		if bErr != nil {
			fmt.Println("Error ", bErr)
			return
		}

		mErr := json.Unmarshal(b, &artists)
		if mErr != nil {
			fmt.Println("Error ", mErr)
			return
		}

		for i := 0; i < len(artists.Items); i++ {
			go driverWorkGenre(client, artists.Items[i].Genre)
			clean, cErr := json.Marshal(artists.Items[i])
			if cErr != nil {
				fmt.Println("Error ", cErr)
				return
			}
			key := "Pos" + strconv.Itoa(i)
			fmt.Println("TOP Artist writing", key, string(clean))
			err := storeClient.KVJSON.Write("SpotifyTopArtists", key, clean)
			if err != nil {
				libDatabox.Err("Error Write Datasource " + err.Error())
			}
		}
	}
}

func driverWorkGenre(client spotify.Client, info []string) {
	genres := make([]Genres, 0)

	for i := 0; i < len(info); i++ {
		for j := 0; j < len(genres); j++ {
			if info[i] == genres[j].Name {
				genres[j].Count++
			}
		}
		var temp Genres
		temp.Name = info[i]
		temp.Count++
		genres = append(genres, temp)
	}

	//Sort genres from most popular to least, based on count
	sort.Slice(genres, func(i, j int) bool {
		return genres[i].Count > genres[j].Count
	})

	//Store genre name in store based on popularity
	for k := 0; k < len(genres); k++ {
		key := "Pos" + strconv.Itoa(k)
		err := storeClient.KVJSON.Write("SpotifyTopGenres", key, []byte(`{"genre":"`+genres[k].Name+`"}`))
		if err != nil {
			libDatabox.Err("Error Write Datasource " + err.Error())
		}
	}

}

func setUpWebServer(testMode bool, r *mux.Router, port string) {

	//Start up a well behaved HTTP/S server for displying the UI

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
		Handler:      r,
	}
	if testMode {
		//set up an http server for testing
		libDatabox.Info("Waiting for http requests on port http://127.0.0.1" + srv.Addr)
		log.Fatal(srv.ListenAndServe())
	} else {
		//configure tls
		tlsConfig := &tls.Config{
			PreferServerCipherSuites: true,
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
			},
		}
		srv.TLSConfig = tlsConfig

		libDatabox.Info("Waiting for https requests on port " + srv.Addr)
		log.Fatal(srv.ListenAndServeTLS(libDatabox.GetHttpsCredentials(), libDatabox.GetHttpsCredentials()))
	}
}
