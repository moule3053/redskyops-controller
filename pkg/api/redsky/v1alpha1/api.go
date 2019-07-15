/*
Copyright 2019 GramLabs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1alpha1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gramLabs/k8s-experiment/pkg/api"
)

const (
	endpointExperiment = "/experiments"

	relationSelf      = "self"
	relationNext      = "next"
	relationPrev      = "prev"
	relationPrevious  = "previous"
	relationTrials    = "https://carbonrelay.com/rel/trials"
	relationNextTrial = "https://carbonrelay.com/rel/nextTrial"
)

// Meta is used to collect resource metadata from the response
type Meta interface {
	SetLocation(string)
	SetLastModified(time.Time)
	SetLink(rel, link string)
}

type ErrorType string

const (
	ErrExperimentNameInvalid  ErrorType = "experiment-name-invalid"
	ErrExperimentNameConflict           = "experiment-name-conflict"
	ErrExperimentInvalid                = "experiment-invalid"
	ErrExperimentNotFound               = "experiment-not-found"
	ErrExperimentStopped                = "experiment-stopped"
	ErrTrialInvalid                     = "trial-invalid"
	ErrTrialUnavailable                 = "trial-unavailable"
	ErrTrialNotFound                    = "trial-not-found"
)

// Error represents the API specific error messages and may be used in response to HTTP status codes
type Error struct {
	Type       ErrorType
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s", e.Type)
}

// ExperimentName exists to clearly separate cases where an actual name can be used
type ExperimentName interface {
	Name() string
}

// NewExperimentName returns an experiment name for a given string
func NewExperimentName(n string) ExperimentName {
	return experimentName{name: n}
}

type experimentName struct {
	name string
}

func (n experimentName) Name() string {
	return n.name
}

func (n experimentName) String() string {
	return n.name
}

type Optimization struct {
	// The estimated number of trial runs to perform for an experiment.
	ExperimentBudget int32 `json:"experimentBudget,omitempty"`
	// The total number of concurrent trial runs supported for an experiment.
	ParallelTrials int32 `json:"parallelTrials,omitempty"`
	// The total number of random trials used to start an experiment.
	BurnIn int32 `json:"burnIn,omitempty"`
}

type Metric struct {
	// The name of the metric.
	Name string `json:"name"`
	// The flag indicating this metric should be minimized.
	Minimize bool `json:"minimize,omitempty"`
}

type ParameterType string

const (
	ParameterTypeInteger ParameterType = "int"
	ParameterTypeDouble                = "double"
)

type Bounds struct {
	// The minimum value for a numeric parameter.
	Min json.Number `json:"min"`
	// The maximum value for a numeric parameter.
	Max json.Number `json:"max"`
}

// Parameter is a variable that is going to be tuned in an experiment
type Parameter struct {
	// The name of the parameter.
	Name string `json:"name"`
	// The type of the parameter.
	Type ParameterType `json:"type"`
	// The domain of the parameter.
	Bounds Bounds `json:"bounds"`
}

type ExperimentMeta struct {
	Self      string `json:"-"`
	Trials    string `json:"-"`
	NextTrial string `json:"-"`
}

func (m *ExperimentMeta) SetLocation(string)        {}
func (m *ExperimentMeta) SetLastModified(time.Time) {}
func (m *ExperimentMeta) SetLink(rel, link string) {
	switch rel {
	case relationSelf:
		m.Self = link
	case relationTrials:
		m.Trials = link
	case relationNextTrial:
		m.NextTrial = link
	}
}

// Experiment combines the search space, outcomes and optimization configuration
type Experiment struct {
	ExperimentMeta

	// The display name of the experiment. Do not use for generating URLs!
	DisplayName string `json:"displayName,omitempty"`
	// Controls how the optimizer will generate trials.
	Optimization Optimization `json:"optimization,omitempty"`
	// The metrics been optimized in the experiment.
	Metrics []Metric `json:"metrics"`
	// The search space of the experiment.
	Parameters []Parameter `json:"parameters"`
}

type ExperimentItem struct {
	Experiment
	// The absolute URL used to reference the individual experiment.
	ItemRef string `json:"itemRef,omitempty"`
}

type ExperimentListMeta struct {
	Next string `json:"-"`
	Prev string `json:"-"`
}

func (m *ExperimentListMeta) SetLocation(string)        {}
func (m *ExperimentListMeta) SetLastModified(time.Time) {}
func (m *ExperimentListMeta) SetLink(rel, link string) {
	switch rel {
	case relationNext:
		m.Next = link
	case relationPrev, relationPrevious:
		m.Prev = link
	}
}

type ExperimentListQuery struct {
	Offset int
	Limit  int
}

func (p *ExperimentListQuery) Encode() string {
	q := url.Values{}
	if p != nil && p.Offset != 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if p != nil && p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	return q.Encode()
}

type ExperimentList struct {
	ExperimentListMeta

	// The list of experiments.
	Experiments []ExperimentItem `json:"experiments,omitempty"`
}

type TrialMeta struct {
	ReportTrial string `json:"-"`
}

func (m *TrialMeta) SetLocation(location string) { m.ReportTrial = location }
func (m *TrialMeta) SetLastModified(time.Time)   {}
func (m *TrialMeta) SetLink(string, string)      {}

type Assignment struct {
	// The name of the parameter in the experiment the assignment corresponds to.
	ParameterName string `json:"parameterName"`
	// The assigned value of the parameter.
	Value json.Number `json:"value"`
}

type TrialAssignments struct {
	TrialMeta

	// The list of parameter names and their assigned values.
	Assignments []Assignment `json:"assignments"`
}

type Value struct {
	// The name of the metric in the experiment the value corresponds to.
	MetricName string `json:"metricName"`
	// The observed value of the metric.
	Value float64 `json:"value"`
	//The observed error of the metric.
	Error float64 `json:"error,omitempty"`
}

type TrialValues struct {
	// The observed values.
	Values []Value `json:"values,omitempty"`
	// Indicator that the trial failed, Values is ignored when true.
	Failed bool `json:"failed,omitempty"`
}

type TrialStatus string

const (
	TrialStaged    TrialStatus = "staged"
	TrialActive                = "active"
	TrialCompleted             = "completed"
	TrialFailed                = "failed"
)

type TrialItem struct {
	TrialAssignments
	TrialValues

	// The current trial status.
	Status TrialStatus `json:"status"`
	// Labels for this trial.
	Labels map[string]string `json:"labels"`
}

type TrialList struct {
	// The list of trials.
	Trials []TrialItem `json:"trials"`
}

// API provides bindings for the supported endpoints
type API interface {
	GetAllExperiments(context.Context, *ExperimentListQuery) (ExperimentList, error)
	GetAllExperimentsByPage(context.Context, string) (ExperimentList, error)
	GetExperimentByName(context.Context, ExperimentName) (Experiment, error)
	GetExperiment(context.Context, string) (Experiment, error)
	CreateExperiment(context.Context, ExperimentName, Experiment) (Experiment, error)
	DeleteExperiment(context.Context, string) error
	GetAllTrials(context.Context, string) (TrialList, error)
	CreateTrial(context.Context, string, TrialAssignments) (string, error) // TODO Should this return TrialAssignments?
	NextTrial(context.Context, string) (TrialAssignments, error)
	ReportTrial(context.Context, string, TrialValues) error
}

// NewApi returns a new version specific API for the specified client
func NewApi(c api.Client) API {
	return &httpAPI{client: c}
}

// NewForConfig returns a new version specific API for the specified client configuration
func NewForConfig(c *api.Config) (API, error) {
	client, err := api.NewClient(*c)
	if err != nil {
		return nil, err
	}
	return NewApi(client), nil
}

type httpAPI struct {
	client api.Client
}

func (h *httpAPI) GetAllExperiments(ctx context.Context, q *ExperimentListQuery) (ExperimentList, error) {
	u := h.client.URL(endpointExperiment)
	u.RawQuery = q.Encode()
	return h.GetAllExperimentsByPage(ctx, u.String())
}

func (h *httpAPI) GetAllExperimentsByPage(ctx context.Context, u string) (ExperimentList, error) {
	lst := ExperimentList{}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return lst, err
	}

	resp, body, err := h.client.Do(ctx, req)
	if err != nil {
		return lst, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		metaUnmarshal(resp.Header, &lst.ExperimentListMeta)
		err = json.Unmarshal(body, &lst)
		return lst, nil
	default:
		return lst, unexpected(resp)
	}
}

func (h *httpAPI) GetExperimentByName(ctx context.Context, n ExperimentName) (Experiment, error) {
	u := h.client.URL(endpointExperiment + "/" + url.PathEscape(n.Name()))
	return h.GetExperiment(ctx, u.String())
}

func (h *httpAPI) GetExperiment(ctx context.Context, u string) (Experiment, error) {
	e := Experiment{}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return e, err
	}

	resp, body, err := h.client.Do(ctx, req)
	if err != nil {
		return e, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		metaUnmarshal(resp.Header, &e.ExperimentMeta)
		err = json.Unmarshal(body, &e)
		return e, err
	case http.StatusNotFound:
		return e, &Error{Type: ErrExperimentNotFound}
	default:
		return e, unexpected(resp)
	}
}

func (h *httpAPI) CreateExperiment(ctx context.Context, n ExperimentName, exp Experiment) (Experiment, error) {
	e := Experiment{}
	u := h.client.URL(endpointExperiment + "/" + url.PathEscape(n.Name()))

	body, err := json.Marshal(exp)
	if err != nil {
		return e, err
	}

	req, err := http.NewRequest(http.MethodPut, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return e, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, _, err := h.client.Do(ctx, req)
	if err != nil {
		return e, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		metaUnmarshal(resp.Header, &e.ExperimentMeta)
		err = json.Unmarshal(body, &e)
		return e, nil
	case http.StatusCreated:
		metaUnmarshal(resp.Header, &e.ExperimentMeta)
		err = json.Unmarshal(body, &e)
		return e, nil
	case http.StatusBadRequest:
		return e, &Error{Type: ErrExperimentNameInvalid}
	case http.StatusConflict:
		return e, &Error{Type: ErrExperimentNameConflict}
	case http.StatusUnprocessableEntity:
		return e, &Error{Type: ErrExperimentInvalid}
	default:
		return e, unexpected(resp)
	}
}

func (h *httpAPI) DeleteExperiment(ctx context.Context, u string) error {
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}

	resp, _, err := h.client.Do(ctx, req)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return &Error{Type: ErrExperimentNotFound}
	default:
		return unexpected(resp)
	}
}

func (h *httpAPI) GetAllTrials(ctx context.Context, u string) (TrialList, error) {
	lst := TrialList{}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return lst, err
	}

	resp, body, err := h.client.Do(ctx, req)
	if err != nil {
		return lst, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		err = json.Unmarshal(body, &lst)
		return lst, nil
	default:
		return lst, unexpected(resp)
	}
}

func (h *httpAPI) CreateTrial(ctx context.Context, u string, asm TrialAssignments) (string, error) {
	l := ""

	body, err := json.Marshal(asm)
	if err != nil {
		return l, err
	}

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBuffer(body))
	if err != nil {
		return l, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, _, err := h.client.Do(ctx, req)
	if err != nil {
		return l, err
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		l = resp.Header.Get("Location")
		return l, nil
	case http.StatusUnprocessableEntity:
		return "", &Error{Type: ErrTrialInvalid}
	default:
		return l, unexpected(resp)
	}
}

func (h *httpAPI) NextTrial(ctx context.Context, u string) (TrialAssignments, error) {
	asm := TrialAssignments{}

	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return asm, err
	}

	resp, body, err := h.client.Do(ctx, req)
	if err != nil {
		return asm, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		metaUnmarshal(resp.Header, &asm.TrialMeta)
		err = json.Unmarshal(body, &asm)
		return asm, err
	case http.StatusGone:
		return asm, &Error{Type: ErrExperimentStopped}
	case http.StatusServiceUnavailable:
		// TODO We should include the retry logic here or at the HTTP client
		ra, err := strconv.Atoi(resp.Header.Get("Retry-After"))
		if err != nil {
			ra = 5
		}
		return asm, &Error{Type: ErrTrialUnavailable, RetryAfter: time.Duration(ra) * time.Second}
	default:
		return asm, unexpected(resp)
	}
}

func (h *httpAPI) ReportTrial(ctx context.Context, u string, vls TrialValues) error {
	if vls.Failed {
		vls.Values = nil
	}

	body, err := json.Marshal(vls)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, _, err := h.client.Do(ctx, req)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusNotFound:
		return &Error{Type: ErrTrialNotFound}
	case http.StatusUnprocessableEntity:
		return &Error{Type: ErrTrialInvalid}
	default:
		return unexpected(resp)
	}
}

func unexpected(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized")
	}
	return fmt.Errorf("unexpected server response: %d", resp.StatusCode)
}

// Extract metadata from the response headers, failures are silently ignored, always call before extracting entity body
func metaUnmarshal(header http.Header, meta Meta) {
	if location := header.Get("Location"); location != "" {
		meta.SetLocation(location)
	}

	if text := header.Get("Last-Modified"); text != "" {
		if lastModified, err := http.ParseTime(text); err == nil {
			meta.SetLastModified(lastModified)
		}
	}

	for _, rh := range header[http.CanonicalHeaderKey("Link")] {
		for _, h := range strings.Split(rh, ",") {
			var link, rel string
			for _, l := range strings.Split(h, ";") {
				l = strings.Trim(l, " ")
				if l == "" {
					continue
				}

				if l[0] == '<' && l[len(l)-1] == '>' {
					link = strings.Trim(l, "<>")
					continue
				}

				p := strings.SplitN(l, "=", 2)
				if len(p) == 2 && strings.ToLower(p[0]) == "rel" {
					rel = strings.Trim(p[1], "\"")
					continue
				}
			}
			meta.SetLink(rel, link)
		}
	}
}
