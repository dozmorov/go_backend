package lib

import (
	"net/http"
	"github.com/gorilla/mux"
)

var controller = &Controller{}
type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}
type Routes []Route

var routes = Routes {
	Route {
		"Index",
		"GET",
		"/",controller.IndexHandler,
	},
	Route {
		"Index",
		"POST",
		"/login",controller.LoginHandler,
	},
	Route {
		"Index",
		"POST",
		"/add_user",controller.AddUserHandler,
	},
	Route {
		"Index",
		"POST",
		"/",controller.IndexHandler,
	},
}

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		var handler http.Handler
		handler = route.HandlerFunc
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}
	return router
}