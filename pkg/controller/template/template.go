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
package template

import (
	"bytes"
	"fmt"
	"math"
	"text/template"
	"time"

	redskyv1alpha1 "github.com/redskyops/k8s-experiment/pkg/apis/redsky/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// PatchData represents a trial during patch evaluation
type PatchData struct {
	// Trial metadata
	Trial metav1.ObjectMeta
	// Trial assignments
	Values map[string]int64
}

// MetricData represents a trial during metric evaluation
type MetricData struct {
	// Trial metadata
	Trial metav1.ObjectMeta
	// The time at which the trial run started (possibly adjusted)
	StartTime time.Time
	// The time at which the trial run completed
	CompletionTime time.Time
	// The duration of the trial run expressed as a Prometheus range value
	Range string
}

func NewPatchData(t *redskyv1alpha1.Trial) *PatchData {
	d := &PatchData{}

	t.ObjectMeta.DeepCopyInto(&d.Trial)

	for _, a := range t.Spec.Assignments {
		d.Values[a.Name] = a.Value
	}

	return d
}

func NewMetricData(t *redskyv1alpha1.Trial) *MetricData {
	d := &MetricData{}

	t.ObjectMeta.DeepCopyInto(&d.Trial)

	// Override the namespace with the target namespace
	if t.Spec.TargetNamespace != "" {
		d.Trial.Namespace = t.Spec.TargetNamespace
	}

	if t.Status.StartTime != nil {
		d.StartTime = t.Status.StartTime.Time
	}

	if t.Status.CompletionTime != nil {
		d.CompletionTime = t.Status.CompletionTime.Time
	}

	d.Range = fmt.Sprintf("%.0fs", math.Max(d.CompletionTime.Sub(d.StartTime).Seconds(), 0))

	return d
}

type TemplateEngine struct {
	FuncMap template.FuncMap
}

func NewTemplateEngine() *TemplateEngine {
	f := FuncMap()
	return &TemplateEngine{
		FuncMap: f,
	}
}

// TODO Investigate better use of template names
// Would it be possible to have the template engine hold more scope? e.g. create the template engine using the full list
// of patch templates or metrics (or the experiment itself, trial for HelmValues) and then render the individual values by template name?

// RenderPatch returns the JSON representation of the supplied patch template (input can be a Go template that produces YAML)
func (e *TemplateEngine) RenderPatch(patch *redskyv1alpha1.PatchTemplate, trial *redskyv1alpha1.Trial) ([]byte, error) {
	data := NewPatchData(trial)
	b, err := e.render("patch", patch.Patch, data) // TODO What should we use for patch template names? Something from the targetRef?
	if err != nil {
		return nil, err
	}
	return yaml.ToJSON(b.Bytes())
}

// RenderHelmValue returns a "name=value" string of the supplied Helm value
func (e *TemplateEngine) RenderHelmValue(helmValue *redskyv1alpha1.HelmValue, trial *redskyv1alpha1.Trial) (string, error) {
	data := NewPatchData(trial)
	b, err := e.render(helmValue.Name, helmValue.Value.String(), data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s=%s", helmValue.Name, b.String()), nil
}

// RenderMetricQueries returns the metric query and the metric error query
func (e *TemplateEngine) RenderMetricQueries(metric *redskyv1alpha1.Metric, trial *redskyv1alpha1.Trial) (string, string, error) {
	data := NewMetricData(trial)
	b1, err := e.render(metric.Name, metric.Query, data)
	if err != nil {
		return "", "", nil
	}
	b2, err := e.render(metric.Name, metric.ErrorQuery, data)
	if err != nil {
		return "", "", nil
	}
	return b1.String(), b2.String(), nil
}

func (e *TemplateEngine) render(name, text string, data interface{}) (*bytes.Buffer, error) {
	tmpl, err := template.New(name).Funcs(e.FuncMap).Parse(text)
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	if err = tmpl.Execute(b, data); err != nil {
		return nil, err
	}
	return b, nil
}