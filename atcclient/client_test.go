package atcclient_test

import (
	"net/http"

	"github.com/concourse/atc"
	. "github.com/concourse/fly/atcclient"
	"github.com/concourse/fly/rc"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("ATC Client", func() {
	var (
		api      string
		username string
		password string
		cert     string
		insecure bool
	)

	BeforeEach(func() {
		api = "f"
		username = ""
		password = ""
		cert = ""
		insecure = false
	})

	Describe("#NewClient", func() {
		It("Returns back an ATC Client", func() {
			target := rc.NewTarget(api, username, password, cert, insecure)
			client, err := NewClient(target)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("Errors when passed target props with an invalid url", func() {
			target := rc.NewTarget("", username, password, cert, insecure)
			_, err := NewClient(target)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("API is blank"))
		})
	})

	Describe("#Send", func() {
		var (
			atcServer *ghttp.Server
			client    Client
		)

		BeforeEach(func() {
			var err error
			atcServer = ghttp.NewServer()
			client, err = NewClient(
				rc.NewTarget(atcServer.URL(), "", "", "", false),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			atcServer.Close()
		})

		It("Makes a request to the given route", func() {
			expectedURL := "/api/v1/builds/foo"
			atcServer.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", expectedURL),
					ghttp.RespondWithJSONEncoded(200, atc.Build{}, http.Header{atc.ConfigVersionHeader: {"42"}}),
				),
			)
			var build atc.Build
			err := client.Send(Request{
				RequestName: atc.GetBuild,
				Params:      map[string]string{"build_id": "foo"},
				Result:      &build,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(atcServer.ReceivedRequests())).To(Equal(1))
		})

		It("Makes a request with the given parameters to the given route", func() {
			expectedURL := "/api/v1/containers"
			expectedResponse := []atc.Container{
				{
					ID:           "first-container",
					PipelineName: "my-special-pipeline",
					Type:         "check",
					Name:         "bob",
					BuildID:      1,
					WorkerName:   "abc",
				},
				{
					ID:           "second-container",
					PipelineName: "my-special-pipeline",
					Type:         "check",
					Name:         "alice",
					BuildID:      1,
					WorkerName:   "def",
				},
			}
			atcServer.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", expectedURL, "type=check"),
					ghttp.RespondWithJSONEncoded(200, expectedResponse, http.Header{atc.ConfigVersionHeader: {"42"}}),
				),
			)
			var containers []atc.Container
			err := client.Send(Request{
				Result:      &containers,
				RequestName: atc.ListContainers,
				Queries:     map[string]string{"type": "check"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(atcServer.ReceivedRequests())).To(Equal(1))
			Expect(containers).To(Equal(expectedResponse))
		})

		Describe("Headers", func() {
			BeforeEach(func() {
				var err error
				atcServer = ghttp.NewServer()

				username = "foo"
				password = "bar"
				target := rc.NewTarget(atcServer.URL(), username, password, cert, insecure)
				client, err = NewClient(target)
				Expect(err).NotTo(HaveOccurred())

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/api/v1/builds/foo"),
						ghttp.VerifyBasicAuth(username, password),
						ghttp.RespondWithJSONEncoded(200, atc.Build{}, http.Header{atc.ConfigVersionHeader: {"42"}}),
					),
				)

			})

			It("Sets the username and password if given", func() {
				var build atc.Build
				err := client.Send(Request{
					RequestName: atc.GetBuild,
					Params:      map[string]string{"build_id": "foo"},
					Result:      &build,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Different status codes", func() {
			Describe("204 no content", func() {
				BeforeEach(func() {
					var err error
					atcServer = ghttp.NewServer()

					target := rc.NewTarget(atcServer.URL(), username, password, cert, insecure)
					client, err = NewClient(target)
					Expect(err).NotTo(HaveOccurred())

					atcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("DELETE", "/api/v1/pipelines/foo"),
							ghttp.RespondWith(http.StatusNoContent, ""),
						),
					)

				})

				It("Sets the username and password if given", func() {
					err := client.Send(Request{
						RequestName: atc.DeletePipeline,
						Params:      map[string]string{"pipeline_name": "foo"},
					})
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Describe("Non-2XX response", func() {
				BeforeEach(func() {
					var err error
					atcServer = ghttp.NewServer()

					target := rc.NewTarget(atcServer.URL(), username, password, cert, insecure)
					client, err = NewClient(target)
					Expect(err).NotTo(HaveOccurred())

					atcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("DELETE", "/api/v1/pipelines/foo"),
							ghttp.RespondWith(http.StatusInternalServerError, "problem"),
						),
					)

				})

				It("returns back UnexpectedResponseError", func() {
					err := client.Send(Request{
						RequestName: atc.DeletePipeline,
						Params:      map[string]string{"pipeline_name": "foo"},
					})
					Expect(err).To(HaveOccurred())
					ure, ok := err.(UnexpectedResponseError)
					Expect(ok).To(BeTrue())
					Expect(ure.StatusCode).To(Equal(http.StatusInternalServerError))
					Expect(ure.Body).To(Equal("problem"))
				})
			})
		})

		Describe("Request Body", func() {
			var plan atc.Plan

			BeforeEach(func() {
				plan = atc.Plan{
					OnSuccess: &atc.OnSuccessPlan{
						Step: atc.Plan{
							Aggregate: &atc.AggregatePlan{},
						},
						Next: atc.Plan{
							Location: &atc.Location{
								ID:       4,
								ParentID: 0,
							},
							Task: &atc.TaskPlan{
								Name:       "one-off",
								Privileged: true,
								Config:     &atc.TaskConfig{},
							},
						},
					},
				}

				expectedURL := "/api/v1/builds"

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", expectedURL),
						ghttp.VerifyJSONRepresenting(plan),
						ghttp.RespondWith(http.StatusNoContent, ""),
					),
				)
			})

			It("serializes the given body and sends it in the request body", func() {
				err := client.Send(Request{
					RequestName: atc.CreateBuild,
					Body:        plan,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
