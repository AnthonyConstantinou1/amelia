package amelia

import (
	"net/http"

	"appengine"
	"appengine/datastore"
	"appengine/user"
)

type PhoneEntry struct {
	Parent string
	Phone  string
}

func addPhone(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Redirect(w, r, "/", http.StatusUnauthorized)
		return nil
	}
	_, err := datastore.Put(c, datastore.NewKey(c, "PhoneEntry", r.FormValue("parent"), 0, datastore.NewKey(c, "User", u.ID, 0, nil)), &PhoneEntry{
		Parent: r.FormValue("parent"),
		Phone:  r.FormValue("phone"),
	})
	if err != nil {
		return &appError{err, "Error saving phone", http.StatusInternalServerError}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func delPhone(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Redirect(w, r, "/", http.StatusUnauthorized)
		return nil
	}
	err := datastore.Delete(c, datastore.NewKey(c, "PhoneEntry", r.FormValue("parent"), 0, datastore.NewKey(c, "User", u.ID, 0, nil)))
	if err != nil {
		return &appError{err, "Error deleting phone", http.StatusInternalServerError}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}
