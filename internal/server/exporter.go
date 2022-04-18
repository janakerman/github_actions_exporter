package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cpanato/github_actions_exporter/model"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/go-github/v43/github"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"
)

var (
	HistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_execution_time_seconds",
		Help:    "Time that a workflow took to run.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "workflow_name"},
	)

	totalMinutesUsedActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_minutes",
		Help: "Total minutes used for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	includedMinutesUsedActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_included_minutes",
		Help: "Included Minutes for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalPaidMinutesActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_paid_minutes",
		Help: "Paid Minutes for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedUbuntuActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_ubuntu_minutes",
		Help: "Total minutes used for Ubuntu type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedMacOSActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_macos_minutes",
		Help: "Total minutes used for MacOS type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedWindowsActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_windows_minutes",
		Help: "Total minutes used for Windows type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)
)

func init() {
	//Register metrics with prometheus
	prometheus.MustRegister(HistogramVec)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedUbuntuActions)
	prometheus.MustRegister(totalMinutesUsedMacOSActions)
	prometheus.MustRegister(totalMinutesUsedWindowsActions)
}

// CollectWorkflowRun collect the workflow execution run metric
func CollectWorkflowRun(checkRunEvent *github.CheckRunEvent) {
	if checkRunEvent.GetCheckRun().GetStatus() != "completed" {
		return
	}

	workflowName := checkRunEvent.GetCheckRun().GetName()
	repo := checkRunEvent.GetRepo().GetName()
	org := checkRunEvent.GetRepo().GetOwner().GetLogin()
	endTime := checkRunEvent.GetCheckRun().GetCompletedAt()
	startTime := checkRunEvent.GetCheckRun().GetStartedAt()
	executionTime := endTime.Sub(startTime.Time).Seconds()

	HistogramVec.WithLabelValues(org, repo, workflowName).Observe(executionTime)
}

// GHActionExporter struct to hold some information
type GHActionExporter struct {
	GHClient *github.Client
	Logger   log.Logger
	Opts     ServerOpts
}

func NewGHActionExporter(logger log.Logger, opts ServerOpts) *GHActionExporter {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.GitHubAPIToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &GHActionExporter{
		GHClient: client,
		Logger:   logger,
		Opts:     opts,
	}
}

// CollectActionBilling collect the action billing.
func (c *GHActionExporter) CollectActionBilling() {
	if c.Opts.GitHubOrg != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingOrg(context.TODO(), c.Opts.GitHubOrg)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an org", "org", c.Opts.GitHubOrg, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}

	if c.Opts.GitHubUser != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingUser(context.TODO(), c.Opts.GitHubUser)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an user", "user", c.Opts.GitHubUser, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}
}

// handleGHWebHook responds to POST /gh_event, when receive a event from GitHub.
func (c *GHActionExporter) HandleGHWebHook(w http.ResponseWriter, r *http.Request) {
	buf, _ := ioutil.ReadAll(r.Body)

	receivedHash := strings.SplitN(r.Header.Get("X-Hub-Signature"), "=", 2)
	if receivedHash[0] != "sha1" {
		level.Error(c.Logger).Log("msg", "invalid webhook hash signature: SHA1")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := validateSignature(c.Opts.GitHubToken, receivedHash, buf)
	if err != nil {
		level.Error(c.Logger).Log("msg", "invalid token", "err", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "ping":
		pingEvent := model.PingEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		if pingEvent == nil {
			level.Info(c.Logger).Log("msg", "ping event", "hookID", pingEvent.GetHookID())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status": "honk"}`))
		return
	case "check_run":
		event := model.CheckRunEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		level.Info(c.Logger).Log("msg", "got check_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "workflowName", event.GetCheckRun().GetName())
		go CollectWorkflowRun(event)
	case "workflow_job":
		event := model.WorkflowJobEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		level.Info(c.Logger).Log("msg", "got workflow_job event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName())
	default:
		level.Info(c.Logger).Log("msg", "not implemented")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	go c.CollectActionBilling()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

// validateSignature validate the incoming github event.
func validateSignature(gitHubToken string, receivedHash []string, bodyBuffer []byte) error {
	hash := hmac.New(sha1.New, []byte(gitHubToken))
	if _, err := hash.Write(bodyBuffer); err != nil {
		msg := fmt.Sprintf("Cannot compute the HMAC for request: %s\n", err)
		return errors.New(msg)
	}

	expectedHash := hex.EncodeToString(hash.Sum(nil))
	if receivedHash[1] != expectedHash {
		msg := fmt.Sprintf("Expected Hash does not match the received hash: %s\n", expectedHash)
		return errors.New(msg)
	}

	return nil
}
