package in_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/concourse/concourse-pipeline-resource/concourse"
	"github.com/concourse/concourse-pipeline-resource/concourse/api"
	"github.com/concourse/concourse-pipeline-resource/concourse/api/apifakes"
	"github.com/concourse/concourse-pipeline-resource/fly/flyfakes"
	"github.com/concourse/concourse-pipeline-resource/in"
	"github.com/concourse/concourse-pipeline-resource/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/robdimsdale/sanitizer"
)

var _ = Describe("In", func() {
	var (
		downloadDir string

		ginkgoLogger logger.Logger

		target string
		teams  []concourse.Team

		flyBinaryPath string

		inRequest         concourse.InRequest
		pipelinesChecksum string
		inCommand         *in.InCommand

		fakeFlyConn     *flyfakes.FakeFlyConn
		flyRunCallCount int

		fakeAPIClient *apifakes.FakeClient

		pipelines        []api.Pipeline
		pipelineVersions []string

		pipelinesErr error

		pipelineContents []string
	)

	BeforeEach(func() {
		flyRunCallCount = 0
		fakeFlyConn = &flyfakes.FakeFlyConn{}
		fakeAPIClient = &apifakes.FakeClient{}

		var err error
		downloadDir, err = ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())

		target = "some target"
		teams = []concourse.Team{
			{
				Name:     "main",
				Username: "some user",
				Password: "some password",
			},
		}
		flyBinaryPath = "fly"

		pipelinesChecksum = "some-checksum"

		pipelinesErr = nil
		pipelines = []api.Pipeline{
			{
				Name: "pipeline-1",
				URL:  "pipeline_URL_1",
			},
			{
				Name: "pipeline-2",
				URL:  "pipeline_URL_2",
			},
		}
		pipelineVersions = []string{"1234", "2345"}
		pipelineContents = make([]string, 2)

		pipelineContents[0] = `---
pipeline1: foo
`

		pipelineContents[1] = `---
pipeline2: foo
`

		inRequest = concourse.InRequest{
			Source: concourse.Source{
				Target: target,
				Teams:  teams,
			},
			Version: concourse.Version{
				pipelines[0].Name: pipelineVersions[0],
			},
		}

		fakeFlyConn.GetPipelineStub = func(name string) ([]byte, error) {
			ginkgoLogger.Debugf("GetPipelineStub for: %s\n", name)

			switch name {
			case pipelines[0].Name:
				return []byte(pipelineContents[0]), nil
			case pipelines[1].Name:
				return []byte(pipelineContents[1]), nil
			default:
				Fail("Unexpected invocation of flyConn.GetPipeline")
				return nil, nil
			}
		}
	})

	JustBeforeEach(func() {
		fakeAPIClient.PipelinesReturns(pipelines, pipelinesErr)

		sanitized := concourse.SanitizedSource(inRequest.Source)
		sanitizer := sanitizer.NewSanitizer(sanitized, GinkgoWriter)

		ginkgoLogger = logger.NewLogger(sanitizer)

		binaryVersion := "v0.1.2-unit-tests"
		inCommand = in.NewInCommand(binaryVersion, ginkgoLogger, fakeFlyConn, fakeAPIClient, downloadDir)
	})

	AfterEach(func() {
		err := os.RemoveAll(downloadDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("downloads all pipeline configs to the target directory", func() {
		_, err := inCommand.Run(inRequest)

		Expect(err).NotTo(HaveOccurred())

		files, err := ioutil.ReadDir(downloadDir)
		Expect(err).NotTo(HaveOccurred())

		Expect(files).To(HaveLen(len(pipelines)))
		Expect(files[0].Name()).To(MatchRegexp("%s.yml", pipelines[0].Name))

		contents, err := ioutil.ReadFile(filepath.Join(downloadDir, files[0].Name()))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal(pipelineContents[0]))

		Expect(files[1].Name()).To(MatchRegexp("%s.yml", pipelines[1].Name))

		contents, err = ioutil.ReadFile(filepath.Join(downloadDir, files[1].Name()))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal(pipelineContents[1]))
	})

	It("returns provided version", func() {
		response, err := inCommand.Run(inRequest)

		Expect(err).NotTo(HaveOccurred())

		Expect(response.Version[pipelines[0].Name]).To(Equal(pipelineVersions[0]))
	})

	It("returns metadata", func() {
		response, err := inCommand.Run(inRequest)

		Expect(err).NotTo(HaveOccurred())

		Expect(response.Metadata).NotTo(BeNil())
	})

	It("syncs the fly version to the given target", func() {
		_, err := inCommand.Run(inRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeFlyConn.SyncCallCount()).To(Equal(1))
	})

	Context("when insecure parses as true", func() {
		BeforeEach(func() {
			inRequest.Source.Insecure = "true"
		})

		It("invokes the login with insecure: true, without error", func() {
			_, err := inCommand.Run(inRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeFlyConn.LoginCallCount()).To(Equal(1))
			_, _, _, _, insecure := fakeFlyConn.LoginArgsForCall(0)

			Expect(insecure).To(BeTrue())
		})
	})

	Context("when insecure fails to parse into a boolean", func() {
		BeforeEach(func() {
			inRequest.Source.Insecure = "unparsable"
		})

		It("returns an error", func() {
			_, err := inCommand.Run(inRequest)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when login returns an error", func() {
		var (
			expectedErr error
		)

		BeforeEach(func() {
			expectedErr = fmt.Errorf("login failed")
			fakeFlyConn.LoginReturns(nil, expectedErr)
		})

		It("returns an error", func() {
			_, err := inCommand.Run(inRequest)
			Expect(err).To(HaveOccurred())

			Expect(err).To(Equal(expectedErr))
		})
	})

	Context("when getting pipelines returns an error", func() {
		BeforeEach(func() {
			pipelinesErr = fmt.Errorf("some error")
		})

		It("returns an error", func() {
			_, err := inCommand.Run(inRequest)
			Expect(err).To(HaveOccurred())

			Expect(err).To(Equal(pipelinesErr))
		})
	})

	Context("when getting pipeline returns an error", func() {
		var (
			expectedErr error
		)

		BeforeEach(func() {
			expectedErr = fmt.Errorf("some error")
			fakeFlyConn.GetPipelineReturns(nil, expectedErr)
		})

		It("returns an error", func() {
			_, err := inCommand.Run(inRequest)
			Expect(err).To(Equal(expectedErr))
		})
	})
})
