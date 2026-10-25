// SPDX-License-Identifier: MIT

package mykit_test

import (
	"context"
	"fmt"
	"log"

	"github.com/mycroblog/my/pkg/mykit"
)

// Stream keeps the event stream open until the context ends, and tells every reconnection, after which the
// events missed in between must be read from the timelines and the notifications.
func ExampleAPI_Stream() {
	api, err := mykit.New(mykit.Config{Server: "https://example.com", TokenSource: mykit.StaticToken("access token")})
	if err != nil {
		log.Fatal(err)
	}
	err = api.Stream(context.Background(), mykit.StreamOptions{
		OnOpen: func(reconnected bool) {
			if reconnected {
				fmt.Println("reconnected: read the home timeline and the notifications again")
			}
		},
		OnEvent: func(e mykit.StreamEvent) {
			switch e := e.(type) {
			case *mykit.UpdateEvent:
				fmt.Println("new post", e.Status.Id)
			case *mykit.NotificationEvent:
				fmt.Println("notification", e.Notification.Type)
			case *mykit.AnnouncementEvent, *mykit.AnnouncementRemovedEvent:
				fmt.Println("the announcements changed")
			case *mykit.UnknownEvent:
				// A newer server sent an event this library does not know.
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
