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

	"github.com/gorilla/mux"
	libDatabox "github.com/me-box/lib-go-databox"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

const (
	RedirectHostInsideDatabox      = "https://spotify-history-driver:8080"
	RedirectHostOutsideDatabox     = "http://127.0.0.1:8080"
	OAuthRedirectURIInsideDatabox  = "https://127.0.0.1/core-ui/ui/view/spotify-history-driver/callback"
	OAuthRedirectURIOutsideDatabox = "https://127.0.0.1/ui/spotify-history-driver/callback"
	testArbiterEndpoint            = "tcp://127.0.0.1:4444"
	testStoreEndpoint              = "tcp://127.0.0.1:5555"
)

var (
	auth            spotify.Authenticator
	state           = "abc123"
	storeClient     *libDatabox.CoreStoreClient
	DataboxTestMode = os.Getenv("DATABOX_VERSION") == ""
	stopChan        chan int
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
	} else {
		DataboxStoreEndpoint = os.Getenv("DATABOX_ZMQ_ENDPOINT")
		storeClient = libDatabox.NewDefaultCoreStoreClient(DataboxStoreEndpoint)
	}

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
			client := auth.NewClient(tok)
			channel := make(chan []string)
			stopChan = make(chan int)
			go driverWorkTrack(client, stopChan)
			go driverWorkArtist(client, channel, stopChan)
			go driverWorkGenre(client, channel, stopChan)
		}()
	}

	//Set client_id and client_secret for the application inside the auth object
	redirectURI := OAuthRedirectURIInsideDatabox
	if DataboxTestMode {
		redirectURI = OAuthRedirectURIOutsideDatabox
	}

	auth = spotify.NewAuthenticator(redirectURI,
		spotify.ScopeUserReadPrivate,
		spotify.ScopeUserReadRecentlyPlayed,
		spotify.ScopeUserTopRead)
	auth.SetAuthInfo("2706f5aa27b646d8835a6a8aca7eba37", "eb8aec62450e4d44a4308f07b82338cb")

	setUpWebServer(DataboxTestMode, router, "8080")
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
		DataSourceType: "playlistData",             //required
		DataSourceID:   "SpotifyTrackData",         //required
		StoreType:      libDatabox.StoreTypeTSBlob, //required
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
		DataSourceType: "topArtists",                  //required
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

func driverWorkTrack(client spotify.Client, stop chan int) {
	var recentTime int64
	var opts spotify.RecentlyPlayedOptions

	opts.Limit = 50
	opts.AfterEpochMs = recentTime

	for {
		results, err := client.PlayerRecentlyPlayedOpt(&opts)
		if err != nil {
			fmt.Println("Error ", err)
			return
		}
		if len(results) > 0 {
			//Get most recent items time and convert to milliseconds
			recentTime = results[0].PlayedAt.Unix() * 1000
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
			}
			libDatabox.Info("Storing data")
		} else {
			libDatabox.Info("No new data")
		}
		//time.Sleep(time.Hour * 2)
		time.Sleep(time.Second * 30)

		//Should we stop?
		select {
		case <-stop:
			return
		default:
			//do continue
		}
	}
}

func driverWorkArtist(client spotify.Client, data chan<- []string, stop chan int) {
	var artists ArtistArray
	for {

		results, err := client.CurrentUsersTopArtists()
		if err != nil {
			fmt.Println("Error ", err)
			return
		}
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
				data <- artists.Items[i].Genre

				clean, cErr := json.Marshal(artists.Items[i])
				if cErr != nil {
					fmt.Println("Error ", cErr)
					return
				}
				key := "Pos" + strconv.Itoa(i)
				err := storeClient.KVText.Write("SpotifyTopArtists", key, clean)
				if err != nil {
					libDatabox.Err("Error Write Datasource " + err.Error())
				}
			}
			data <- []string{"end"}

		}
		//time.Sleep(time.Hour * 24)
		time.Sleep(time.Second * 30)

		//Should we stop?
		select {
		case <-stop:
			return
		default:
			//do continue
		}
	}
}

func driverWorkGenre(client spotify.Client, data <-chan []string, stopChan chan int) {
	var stop bool
	genres := make([]Genres, 0)
	for {
		for {
			info := <-data
			if info[0] == "end" {
				break
			}

			for i := 0; i < len(info); i++ {
				stop = false
				for j := 0; j < len(genres); j++ {

					if info[i] == genres[j].Name {
						genres[j].Count++
						stop = true
					}
				}
				if !stop {
					var temp Genres
					temp.Name = info[i]
					temp.Count++

					genres = append(genres, temp)
				}
			}

		}
		//Sort genres from most popular to least, based on count
		sort.Slice(genres, func(i, j int) bool {
			return genres[i].Count > genres[j].Count
		})
		//Store genre name in store based on popularity
		for k := 0; k < len(genres); k++ {
			key := "Pos" + strconv.Itoa(k)
			err := storeClient.KVText.Write("SpotifyTopGenres", key, []byte(genres[k].Name))
			if err != nil {
				libDatabox.Err("Error Write Datasource " + err.Error())
			}
		}
		//time.Sleep(time.Hour * 24)
		time.Sleep(time.Second * 30)

		//Should we stop?
		select {
		case <-stopChan:
			return
		default:
			//do continue
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
