package amelia

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/subosito/twilio"
	"github.com/zachlatta/go-tomtom"

	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"
)

type Notification struct {
	UserID           int64             `json:"userId"`
	StorylineUpdates []StorylineUpdate `json:"storylineUpdates"`
}

type StorylineUpdate struct {
	// TODO: Change to equivalent of enum
	Reason string `json:"reason"`
}

type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type Place struct {
	Location Location `json:"location"`
	Name     string   `json:"name"`
}

type Segment struct {
	Place     Place       `json:"place"`
	StartTime RFC3339Time `json:"startTime"`
}

type DailySegments struct {
	Segments []Segment `json:"segments"`
}

func handleNotification(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return &appError{err, "Error reading request body", http.StatusBadRequest}
	}

	var notification Notification
	err = json.Unmarshal(body, &notification)
	if err != nil {
		return &appError{err, "Error unmarshalling JSON", http.StatusBadRequest}
	}

	hasDataUpload := false
	for _, update := range notification.StorylineUpdates {
		if update.Reason == "DataUpload" {
			hasDataUpload = true
			break
		}
	}

	if hasDataUpload {
		// TODO: Ensure MovesUserId is unique in datastore
		q := datastore.NewQuery("User").Filter("AuthorizedWithMoves =", true).Filter("MovesUserId =", notification.UserID)

		var users []User
		keys, err := q.GetAll(c, &users)
		if err != nil {
			return &appError{err, "Could not get user from datastore",
				http.StatusInternalServerError}
		}

		if len(users) <= 0 {
			return &appError{err, "User not found", http.StatusNotFound}
		}

		user, key := users[0], keys[0]

		t := CreateTransport(c, user.MovesToken.NormToken())

		dailySegmentsList, err := GetLatestPlaces(t)
		if err != nil {
			return &appError{err, "Could not unmarshal request",
				http.StatusBadRequest}
		}

		return updateDailySegments(c, *dailySegmentsList, user, key)
	}

	return nil
}

func updateDailySegments(c appengine.Context,
	dailySegmentsList []DailySegments, user User,
	userKey *datastore.Key) *appError {
	f := urlfetch.Client(c)
	var phoneEntries []PhoneEntry
	_, err := datastore.NewQuery("PhoneEntry").Ancestor(userKey).GetAll(c, &phoneEntries)
	if err != nil {
		return &appError{err, "Error getting phones from datastore",
			http.StatusInternalServerError}
	}

	segmentsToProcess := make([]Segment, 0)

	// Find segments with StartTimes greater than the user's LastSegmentStartTime
	for _, dailySegments := range dailySegmentsList {
		for _, segment := range dailySegments.Segments {
			if user.LastSegmentStartTime.Time.Before(segment.StartTime.Time) {
				segmentsToProcess = append(segmentsToProcess, segment)
			}
		}
	}

	// If there aren't any, return
	if len(segmentsToProcess) <= 0 {
		return nil
	}

	client := tomtom.NewClient(tomtomKey, f)

	for _, segment := range segmentsToProcess {
		var address string
		if segment.Place.Name != "" {
			address = segment.Place.Name
		} else {
			codes, err := client.Geocode.ReverseGeocode(segment.Place.Location.Lat, segment.Place.Location.Lon)
			if err != nil {
				return &appError{err, "Error reverse geocoding address",
					http.StatusInternalServerError}
			}

			if len(codes) > 0 {
				address = codes[0].FormattedAddress
			} else {
				address = fmt.Sprintf("%f, %f", segment.Place.Location.Lat,
					segment.Place.Location.Lon)
			}
		}

		// send texts
		for _, phone := range phoneEntries {
			appError := sendText(c, "I'm now at "+address+".", phone.Phone)
			if err != nil {
				return appError
			}
		}
	}

	lastSegment := segmentsToProcess[len(segmentsToProcess)-1]
	user.LastSegmentStartTime = lastSegment.StartTime

	_, err = datastore.Put(c, userKey, &user)
	if err != nil {
		return &appError{err, "Error saving user", http.StatusInternalServerError}
	}

	return nil
}

func sendText(a appengine.Context, message string, phone string) *appError {
	f := urlfetch.Client(a)
	c := twilio.NewClient(twilioSid, twilioAuthToken, f)

	var params twilio.MessageParams
	params.Body = message
	_, _, err := c.Messages.Send(twilioPhone, phone, params)
	if err != nil {
		return &appError{err, "Error sending text message",
			http.StatusInternalServerError}
	}

	return nil
}
