package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/src-d/engine/api"
	"github.com/src-d/engine/docker"
)

const (
	startComponentTimeout = 60 * time.Second
)

// Component to be run.
type Component struct {
	Name         string
	Start        docker.StartFunc
	Dependencies []Component
}

// Run the given components if they're not already running. It will recursively
// run all the component dependencies.
func Run(ctx context.Context, cs ...Component) error {
	return run(ctx, cs, make(map[string]struct{}))
}

func run(ctx context.Context, cs []Component, seen map[string]struct{}) error {
	for _, c := range cs {
		if len(c.Dependencies) > 0 {
			if err := run(ctx, c.Dependencies, seen); err != nil {
				return err
			}
		}

		if _, ok := seen[c.Name]; ok {
			continue
		}

		seen[c.Name] = struct{}{}
		_, err := docker.InfoOrStart(ctx, c.Name, c.Start)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) StartComponent(
	ctx context.Context,
	r *api.StartComponentRequest,
) (*api.StartComponentResponse, error) {
	return &api.StartComponentResponse{}, s.startComponentAtPort(ctx, r.Name, int(r.Port))
}

func (s *Server) StopComponent(
	ctx context.Context,
	r *api.StopComponentRequest,
) (*api.StopComponentResponse, error) {
	return &api.StopComponentResponse{}, docker.RemoveContainer(r.Name)
}

func (s *Server) startComponent(ctx context.Context, name string) error {
	return s.startComponentAtPort(ctx, name, -1)
}

func (s *Server) startComponentAtPort(ctx context.Context, name string, port int) error {
	switch name {
	case gitbaseWeb.Name:
		return Run(ctx, Component{
			Name:         gitbaseWeb.Name,
			Start:        createGitbaseWeb(docker.WithPort(port, gitbaseWebPrivatePort)),
			Dependencies: []Component{s.gitbaseComponent()},
		})
	case bblfshWeb.Name:
		return Run(ctx, Component{
			Name:         bblfshWeb.Name,
			Start:        createBblfshWeb(docker.WithPort(port, bblfshWebPrivatePort)),
			Dependencies: []Component{s.bblfshComponent()},
		})
	case bblfshd.Name:
		return Run(ctx, s.bblfshComponent())
	case gitbase.Name:
		return Run(ctx, s.gitbaseComponent())
	default:
		return fmt.Errorf("can't start unknown component %s", name)
	}
}

func (s *Server) gitbaseComponent() Component {
	indexDir := filepath.Join(s.datadir, "gitbase", s.workdirHash)

	logrus.Infof("s.datadir: %s", s.datadir)
	logrus.Infof("Indexdir: %s", indexDir)
	logrus.Infof("Workdir: %s", s.workdir)
	logrus.Infof("Gitbase mount path: %s", gitbaseMountPath)
	logrus.Infof("Gitbase index mount path: %s", gitbaseIndexMountPath)

	return Component{
		Name: gitbase.Name,
		Start: createGitbase(
			docker.WithSharedDirectory(s.workdir, gitbaseMountPath),
			docker.WithSharedDirectory(indexDir, gitbaseIndexMountPath),
			docker.WithPort(gitbasePort, gitbasePort),
		),
		Dependencies: []Component{
			s.bblfshComponent(),
		},
	}
}

func (s *Server) bblfshComponent() Component {
	return Component{
		Name: bblfshd.Name,
		Start: createBbblfshd(
			docker.WithPort(bblfshParsePort, bblfshParsePort),
		),
	}
}
