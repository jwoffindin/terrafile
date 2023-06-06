/*
Copyright 2022 IDT Corp.

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

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/jdxcode/netrc"
	"github.com/jessevdk/go-flags"
	"github.com/nritholtz/stdemuxerhook"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type module struct {
	Source       string   `yaml:"source"`
	Directory    string   `yaml:"directory"`
	Version      string   `yaml:"version"`
	Destinations []string `yaml:"destinations"`
}

var opts struct {
	ModulePath    string `short:"p" long:"module-path" default:"./vendor/modules" description:"File path to install generated terraform modules, if not overridden by 'destinations:' field"`
	TerrafilePath string `short:"f" long:"terrafile-file" default:"./Terrafile" description:"File path to the Terrafile file"`
	Clean         bool   `short:"c" long:"clean" description:"Remove everything from destinations and module path upon fetching module(s)\n !!! WARNING !!! Removes all files and folders in the destinations including non-modules."`
	NetRcFile     string `short:"n" long:"netrc-file" description:"Path to .netrc file, if not specified, will use $HOME/.netrc"`
}

// To be set by goreleaser on build
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	netrcFile *netrc.Netrc
)

func init() {
	// Needed to redirect logrus to proper stream STDOUT vs STDERR
	log.AddHook(stdemuxerhook.New(log.StandardLogger()))
}

func writeFile(prefix string, dstDir string, f *object.File) error {
	// Get reader for file contents
	in, err := f.Blob.Reader()
	if err != nil {
		log.Fatalf("failed to read file %s due to error: %s", f.Name, err)
	}

	// Full path to destination path - f.Name can include subdirectories
	dst := filepath.Join(dstDir, f.Name)

	// Ensure the directory of 'dst' file exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		log.Fatalf("failed to create directory for %s due to error: %s", dst, err)
	}

	// Create dest file
	out, err := os.Create(dst)
	if err != nil {
		log.Fatalf("failed to create file %s due to error: %s", f.Name, err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		log.Fatalf("unable to write file %s due to error: %s", f.Name, err)
	}

	return nil
}

func gitClone(repository string, directory string, version string, moduleName string, destinationDir string) {
	modulePath := filepath.Join(destinationDir, moduleName)
	log.Debugf("[*] Removing previously cloned artifacts at %s", modulePath)
	_ = os.RemoveAll(modulePath)
	log.Printf("[*] Checking out %s of %s", version, repository)

	// Refer to https://github.com/go-git/go-git/blob/master/_examples/azure_devops/main.go
	transport.UnsupportedCapabilities = []capability.Capability{
		capability.ThinPack,
	}

	// extract hostname from `repository` which is a URL (git or https)
	auth := getHostAuth(repository)

	// In-memory clone of repository
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:  repository,
		Auth: auth,
	})
	if err != nil {
		log.Fatalf("failed to clone repository %s due to error: %s", repository, err)
	}

	// Resolve reference, tag or branch name, and then map SHA to commit
	ref := resolveReference(repo, version)
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		log.Fatalf("unable to resolve commit associated with tag %s due to error: %s", version, err)
	}

	// Get (root) tree for the commit
	tree, err := commit.Tree()
	if err != nil {
		log.Fatalf("unable to resolve tree associated with tag %s due to error: %s", version, err)
	}

	// If user specified directory, get sub-tree for that directory
	if directory != "" {
		tree, err = tree.Tree(directory)
		if err != nil {
			log.Fatalf("unable to find %s for version %s due to error: %s", directory, version, err)
		}
	}

	// Write out files to local filesystem (under destinationDir)
	err = tree.Files().ForEach(func(f *object.File) error {
		return writeFile(directory, modulePath, f)
	})

	if err != nil {
		log.Fatalf("unable to write module %s files due to error: %s", destinationDir, err)
	}
}

// getHostAuth returns http.BasicAuth for the given repository. If netrc file is
// available, it will be used to get the credentials.
func getHostAuth(repository string) http.AuthMethod {
	endpoint, err := transport.NewEndpoint(repository)
	if err != nil {
		log.Fatalf("failed to parse repository %s due to error: %s", repository, err)
	}

	if netrcFile != nil {
		machine := netrcFile.Machine(endpoint.Host)
		if machine != nil {
			oauthToken := machine.Get("oauth-token")
			if oauthToken != "" {
				return &http.TokenAuth{Token: oauthToken}
			}
			return &http.BasicAuth{
				Username: machine.Get("login"),
				Password: machine.Get("password"),
			}
		}
	}
	return &http.BasicAuth{}
}

// resolveReference resolves a reference to a commit SHA. The reference can be a tag,
// branch (local or remote), or commit SHA. Go-git doesn't set remote tracking, so only
// master branch is available.
func resolveReference(repo *git.Repository, version string) *plumbing.Reference {
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(version), true)
	if err != nil {
		ref, err = repo.Reference(plumbing.NewRemoteReferenceName("origin", version), true)
		if err != nil {
			ref, err = repo.Reference(plumbing.NewTagReferenceName(version), true)
			if err != nil {
				log.Fatalf("unable to resolve version %s due to error: %s", version, err)
			}
		}
	}
	return ref
}

func main() {
	fmt.Printf("Terrafile: version %v, commit %v, built at %v \n", version, commit, date)
	_, err := flags.Parse(&opts)

	// Invalid choice
	if err != nil {
		log.Errorf("failed to parse flags due to: %s", err)
		os.Exit(1)
	}

	workDirAbsolutePath, err := os.Getwd()
	if err != nil {
		log.Errorf("failed to get working directory absolute path due to: %s", err)
	}

	// Get netrc file path
	netrcPath := opts.NetRcFile
	if netrcPath == "" {
		netrcPath = filepath.Join(os.Getenv("HOME"), ".netrc")
	}

	// Get netrc file if it exists
	if _, err := os.Stat(netrcPath); !errors.Is(err, os.ErrNotExist) {
		netrcFile, err = netrc.Parse(netrcPath)
		if err != nil {
			log.Errorf("failed to parse netrc file %s due to error: %s", netrcPath, err)
		}
	}

	// Read File
	yamlFile, err := os.ReadFile(opts.TerrafilePath)
	if err != nil {
		log.Fatalf("failed to read configuration in file %s due to error: %s", opts.TerrafilePath, err)
	}

	// Parse File
	var config map[string]module
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		log.Fatalf("failed to parse yaml file due to error: %s", err)
	}

	if opts.Clean {
		cleanDestinations(config)
	}

	// Clone modules
	var wg sync.WaitGroup
	_ = os.RemoveAll(opts.ModulePath)
	_ = os.MkdirAll(opts.ModulePath, os.ModePerm)

	for key, mod := range config {
		wg.Add(1)
		go func(m module, key string) {
			defer wg.Done()

			// path to clone module
			cloneDestination := opts.ModulePath
			// list of paths to link module to. empty, unless Destinations are more than 1 location
			var linkDestinations []string

			if m.Destinations != nil && len(m.Destinations) > 0 {
				// set first in Destinations as location to clone to
				cloneDestination = filepath.Join(m.Destinations[0], opts.ModulePath)
				// the rest of Destinations are locations to link module to
				linkDestinations = m.Destinations[1:]
			}

			// create folder to clone into
			if err := os.MkdirAll(cloneDestination, os.ModePerm); err != nil {
				log.Errorf("failed to create folder %s due to error: %s", cloneDestination, err)

				// no reason to continue as failed to create folder
				return
			}

			// clone repository
			gitClone(m.Source, m.Directory, m.Version, key, cloneDestination)

			for _, d := range linkDestinations {
				// the source location as folder where module was cloned and module folder name
				moduleSrc := filepath.Join(workDirAbsolutePath, cloneDestination, key)
				// append destination path with module path
				dst := filepath.Join(d, opts.ModulePath)

				log.Infof("[*] Creating folder %s", dst)
				if err := os.MkdirAll(dst, os.ModePerm); err != nil {
					log.Errorf("failed to create folder %s due to error: %s", dst, err)
					return
				}

				dst = filepath.Join(dst, key)

				log.Infof("[*] Remove existing artifacts at %s", dst)
				if err := os.RemoveAll(dst); err != nil {
					log.Errorf("failed to remove location %s due to error: %s", dst, err)
					return
				}

				log.Infof("[*] Link %s to %s", moduleSrc, dst)
				if err := os.Symlink(moduleSrc, dst); err != nil {
					log.Errorf("failed to link module from %s to %s due to error: %s", moduleSrc, dst, err)
				}
			}
		}(mod, key)
	}

	wg.Wait()
}

func cleanDestinations(config map[string]module) {

	// Map filters duplicate destinations with key being each destination's file path
	uniqueDestinations := make(map[string]bool)

	// Range over config and gather all unique destinations
	for _, m := range config {
		if len(m.Destinations) == 0 {
			uniqueDestinations[opts.ModulePath] = true
			continue
		}

		// range over Destinations and put them into map
		for _, dst := range m.Destinations {
			// Destination supposed to be conjunction of destination defined in file with module path
			d := filepath.Join(dst, opts.ModulePath)
			uniqueDestinations[d] = true
		}
	}

	for dst := range uniqueDestinations {

		log.Infof("[*] Removing artifacts from %s", dst)
		if err := os.RemoveAll(dst); err != nil {
			log.Errorf("Failed to remove artifacts from %s due to error: %s", dst, err)
		}
	}
}
