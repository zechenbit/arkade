package get

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/alexellis/arkade/pkg/env"
)

type Tool struct {
	Name           string
	Repo           string
	Owner          string
	Version        string
	URLTemplate    string
	BinaryTemplate string
	NoExtension    bool
}

func (tool Tool) IsArchive() bool {
	arch, operatingSystem := env.GetClientArch()
	version := ""

	downloadURL, _ := GetDownloadURL(&tool, strings.ToLower(operatingSystem), strings.ToLower(arch), version)
	return strings.HasSuffix(downloadURL, "tar.gz") || strings.HasSuffix(downloadURL, "zip")
}

var templateFuncs = map[string]interface{}{
	"HasPrefix": func(s, prefix string) bool { return strings.HasPrefix(s, prefix) },
}

// Download fetches the download URL for a release of a tool
// for a given os,  architecture and version
func GetDownloadURL(tool *Tool, os, arch, version string) (string, error) {
	ver := tool.Version
	if len(version) > 0 {
		ver = version
	}

	dlURL, err := tool.GetURL(os, arch, ver)
	if err != nil {
		return "", err
	}

	return dlURL, nil
}

func (tool Tool) GetURL(os, arch, version string) (string, error) {

	if len(tool.URLTemplate) == 0 {
		return getURLByGithubTemplate(tool, os, arch, version)
	}

	return getByDownloadTemplate(tool, os, arch, version)

}

func (t Tool) Latest() bool {
	return len(t.Version) == 0
}

func getURLByGithubTemplate(tool Tool, os, arch, version string) (string, error) {
	if len(version) == 0 {
		releases := fmt.Sprintf("https://github.com/%s/%s/releases/latest", tool.Owner, tool.Name)
		var err error
		version, err = findGitHubRelease(releases)
		if err != nil {
			return "", err
		}
	}

	var err error
	t := template.New(tool.Name + "binary")

	t = t.Funcs(templateFuncs)
	t, err = t.Parse(tool.BinaryTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	pref := map[string]string{
		"OS":   os,
		"Arch": arch,
		"Name": tool.Name,
	}

	err = t.Execute(&buf, pref)
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(buf.String())

	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/%s/%s",
		tool.Owner, tool.Name, version, res), nil
}

func findGitHubRelease(url string) (string, error) {
	timeout := time.Second * 5
	client := makeHTTPClient(&timeout, false)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	if res.StatusCode != 302 {
		return "", fmt.Errorf("incorrect status code: %d", res.StatusCode)
	}

	loc := res.Header.Get("Location")
	if len(loc) == 0 {
		return "", fmt.Errorf("unable to determine release of tool")
	}

	version := loc[strings.LastIndex(loc, "/")+1:]
	return version, nil
}

func getByDownloadTemplate(tool Tool, os, arch, version string) (string, error) {
	var err error
	t := template.New(tool.Name)
	t = t.Funcs(templateFuncs)
	t, err = t.Parse(tool.URLTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, map[string]string{
		"OS":      os,
		"Arch":    arch,
		"Version": version,
	})
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(buf.String())
	return res, nil
}

func MakeTools() []Tool {
	tools := []Tool{
		{
			Owner: "openfaas",
			Repo:  "faas-cli",
			Name:  "faas-cli",
			BinaryTemplate: `{{ if HasPrefix .OS "ming" -}}
{{.Name}}.exe
{{- else if eq .OS "darwin" -}}
{{.Name}}-darwin
{{- else if eq .Arch "armv6l" -}}
{{.Name}}-armhf
{{- else if eq .Arch "armv7l" -}}
{{.Name}}-armhf
{{- else if eq .Arch "aarch64" -}}
{{.Name}}-arm64
{{- end -}}`,
		},
		//https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/darwin/amd64/kubectl
		{
			Owner:   "kubernetes",
			Repo:    "kubernetes",
			Name:    "kubectl",
			Version: "v1.18.0",
			URLTemplate: `{{$arch := "arm"}}

			{{- if eq .Arch "x86_64" -}}
{{$arch = "amd64"}}
{{- end -}}

{{$ext := ""}}
{{$os := .OS}}

{{ if HasPrefix .OS "ming" -}}
{{$ext = ".exe"}}
{{$os = "windows"}}
{{- end -}}

https://storage.googleapis.com/kubernetes-release/release/{{.Version}}/bin/{{$os}}/{{$arch}}/kubectl{{$ext}}`,
		},
		{
			Owner:       "ahmetb",
			Repo:        "kubectx",
			Name:        "kubectx",
			Version:     "v0.9.0",
			URLTemplate: `https://github.com/ahmetb/kubectx/releases/download/{{.Version}}/kubectx`,
			// Author recommends to keep using Bash version in this release https://github.com/ahmetb/kubectx/releases/tag/v0.9.0
			NoExtension: true,
		},
		{
			Owner:   "helm",
			Repo:    "helm",
			Name:    "helm",
			Version: "v3.2.4",
			URLTemplate: `{{$arch := "arm"}}

{{- if eq .Arch "x86_64" -}}
{{$arch = "amd64"}}
{{- end -}}

{{$os := .OS}}
{{$ext := "tar.gz"}}

{{ if HasPrefix .OS "ming" -}}
{{$os = "windows"}}
{{$ext = "zip"}}
{{- end -}}

https://get.helm.sh/helm-{{.Version}}-{{$os}}-{{$arch}}.{{$ext}}`,
		},
		{
			Owner:   "bitnami-labs",
			Repo:    "sealed-secrets",
			Name:    "kubeseal",
			Version: "v0.12.4",
			URLTemplate: `{{$arch := "arm"}}
{{- if eq .Arch "arm" -}}
https://github.com/bitnami-labs/sealed-secrets/releases/download/{{.Version}}/kubeseal-{{$arch}}
{{- end -}}

{{- if eq .Arch "arm64" -}}
https://github.com/bitnami-labs/sealed-secrets/releases/download/{{.Version}}/kubeseal-{{.Arch}}
{{- end -}}

{{- if HasPrefix .OS "ming" -}}
https://github.com/bitnami-labs/sealed-secrets/releases/download/{{.Version}}/kubeseal.exe
{{- end -}}

{{- if eq .Arch "x86_64" -}}
{{$arch = "amd64"}}
{{- end -}}

{{- if and ( not ( or ( eq $arch "arm") ( eq $arch "arm64")) ) ( or ( eq .OS "darwin" ) ( eq .OS "linux" )) -}}
https://github.com/bitnami-labs/sealed-secrets/releases/download/{{.Version}}/kubeseal-{{.OS}}-{{$arch}}
{{- end -}}`,
		},
	}
	return tools
}

// https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.12.4/kubeseal-linux-amd64
// makeHTTPClient makes a HTTP client with good defaults for timeouts.
func makeHTTPClient(timeout *time.Duration, tlsInsecure bool) http.Client {
	return makeHTTPClientWithDisableKeepAlives(timeout, tlsInsecure, false)
}

// makeHTTPClientWithDisableKeepAlives makes a HTTP client with good defaults for timeouts.
func makeHTTPClientWithDisableKeepAlives(timeout *time.Duration, tlsInsecure bool, disableKeepAlives bool) http.Client {
	client := http.Client{}

	if timeout != nil || tlsInsecure {
		tr := &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: disableKeepAlives,
		}

		if timeout != nil {
			client.Timeout = *timeout
			tr.DialContext = (&net.Dialer{
				Timeout: *timeout,
			}).DialContext

			tr.IdleConnTimeout = 120 * time.Millisecond
			tr.ExpectContinueTimeout = 1500 * time.Millisecond
		}

		if tlsInsecure {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: tlsInsecure}
		}

		tr.DisableKeepAlives = disableKeepAlives

		client.Transport = tr
	}

	return client
}
