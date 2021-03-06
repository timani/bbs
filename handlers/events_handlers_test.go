package handlers_test

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/bbs/events"
	"github.com/cloudfoundry-incubator/bbs/events/eventfakes"
	"github.com/cloudfoundry-incubator/bbs/format"
	"github.com/cloudfoundry-incubator/bbs/handlers"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/bbs/models/test/model_helpers"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/vito/go-sse/sse"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Event Handlers", func() {
	var (
		logger     lager.Logger
		desiredHub events.Hub
		actualHub  events.Hub

		handler         *handlers.EventHandler
		eventStreamDone chan struct{}
		server          *httptest.Server
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		desiredHub = events.NewHub()
		actualHub = events.NewHub()
		handler = handlers.NewEventHandler(logger, desiredHub, actualHub)

		eventStreamDone = make(chan struct{})
	})

	AfterEach(func() {
		desiredHub.Close()
		actualHub.Close()
		server.Close()
	})

	var ItStreamsEventsFromHub = func(hubRef *events.Hub) {
		Describe("Streaming Events", func() {
			var hub events.Hub
			var response *http.Response

			BeforeEach(func() {
				hub = *hubRef
			})

			JustBeforeEach(func() {
				var err error
				response, err = http.Get(server.URL)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when failing to subscribe to the event hub", func() {
				BeforeEach(func() {
					hub.Close()
				})

				It("returns an internal server error", func() {
					Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
				})
			})

			Context("when successfully subscribing to the event hub", func() {
				It("emits events from the hub to the connection", func() {
					reader := sse.NewReadCloser(response.Body)

					hub.Emit(&eventfakes.FakeEvent{Token: "A"})
					encodedPayload := base64.StdEncoding.EncodeToString([]byte("A"))

					Expect(reader.Next()).To(Equal(sse.Event{
						ID:   "0",
						Name: "fake",
						Data: []byte(encodedPayload),
					}))

					hub.Emit(&eventfakes.FakeEvent{Token: "B"})

					encodedPayload = base64.StdEncoding.EncodeToString([]byte("B"))
					Expect(reader.Next()).To(Equal(sse.Event{
						ID:   "1",
						Name: "fake",
						Data: []byte(encodedPayload),
					}))
				})

				It("returns Content-Type as text/event-stream", func() {
					Expect(response.Header.Get("Content-Type")).To(Equal("text/event-stream; charset=utf-8"))
					Expect(response.Header.Get("Cache-Control")).To(Equal("no-cache, no-store, must-revalidate"))
					Expect(response.Header.Get("Connection")).To(Equal("keep-alive"))
				})

				Context("when the source provides an unmarshalable event", func() {
					It("closes the event stream to the client", func() {
						hub.Emit(eventfakes.UnmarshalableEvent{Fn: func() {}})

						reader := sse.NewReadCloser(response.Body)
						_, err := reader.Next()
						Expect(err).To(Equal(io.EOF))
					})
				})

				Context("when the event source returns an error", func() {
					BeforeEach(func() {
						hub.Close()
					})

					It("closes the client event stream", func() {
						reader := sse.NewReadCloser(response.Body)
						_, err := reader.Next()
						Expect(err).To(Equal(io.EOF))
					})
				})

				Context("when the client closes the response body", func() {
					It("returns early", func() {
						reader := sse.NewReadCloser(response.Body)
						hub.Emit(eventfakes.FakeEvent{Token: "A"})
						err := reader.Close()
						Expect(err).NotTo(HaveOccurred())
						Eventually(eventStreamDone, 10).Should(BeClosed())
					})
				})
			})
		})
	}

	Describe("Subscribe_r0", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.Subscribe_r0(w, r)
				close(eventStreamDone)
			}))
		})

		Describe("Subscribe to Desired Events", func() {
			ItStreamsEventsFromHub(&desiredHub)

			It("migrates desired lrps down to v0", func() {
				response, err := http.Get(server.URL)
				Expect(err).NotTo(HaveOccurred())
				reader := sse.NewReadCloser(response.Body)

				desiredLRP := model_helpers.NewValidDesiredLRP("guid")
				event := models.NewDesiredLRPCreatedEvent(desiredLRP)

				migratedLRP := desiredLRP.VersionDownTo(format.V0)
				Expect(migratedLRP).NotTo(Equal(desiredLRP))
				migratedEvent := models.NewDesiredLRPCreatedEvent(migratedLRP)

				expectedEvent, err := events.NewEventFromModelEvent(0, migratedEvent)
				Expect(err).NotTo(HaveOccurred())

				desiredHub.Emit(event)

				Expect(reader.Next()).To(Equal(expectedEvent))
			})
		})

		Describe("Subscribe to Actual Events", func() {
			ItStreamsEventsFromHub(&actualHub)
		})
	})

	Describe("SubscribeToDesiredLRPEvents", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.SubscribeToDesiredLRPEvents(w, r)
				close(eventStreamDone)
			}))
		})

		Describe("Subscribe to Desired Events", func() {
			ItStreamsEventsFromHub(&desiredHub)

			It("does not migrate desired lrps down to v0", func() {
				response, err := http.Get(server.URL)
				Expect(err).NotTo(HaveOccurred())
				reader := sse.NewReadCloser(response.Body)

				desiredLRP := model_helpers.NewValidDesiredLRP("guid")
				event := models.NewDesiredLRPCreatedEvent(desiredLRP)

				desiredHub.Emit(event)

				expectedEvent, err := events.NewEventFromModelEvent(0, event)
				Expect(err).NotTo(HaveOccurred())

				Expect(reader.Next()).To(Equal(expectedEvent))
			})
		})
	})

	Describe("SubscribeToAcutalLRPEvents", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.SubscribeToActualLRPEvents(w, r)
				close(eventStreamDone)
			}))
		})

		Describe("Subscribe to Actual Events", func() {
			ItStreamsEventsFromHub(&actualHub)
		})
	})
})
