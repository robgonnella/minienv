package config

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
)

const defaultImagePlatform = "linux/amd64"

// A "+git" tag is resolved here rather than at deploy time so every chart in
// one run embeds the same sha.
func expandGitTag(
	ctx context.Context,
	svcExtImage *ServiceImage,
	gitClient git.Client,
) error {
	sha, err := gitClient.ShortSha(ctx)
	if err != nil {
		return errs.Errorf(
			ErrGitShortSha,
			"failed to get short sha from git for image tag: %w",
			err,
		)
	}

	svcExtImage.Tag = strings.ReplaceAll(svcExtImage.Tag, "+git", sha)

	return nil
}

const defaultImageTag = "latest"

func parseImageRef(ref string) (string, string, error) {
	if ref == "" {
		return "", "", nil
	}

	// A digest cannot be expressed as repository:tag.
	if strings.Contains(ref, "@") {
		return "", "", errs.Errorf(
			ErrImageDigestUnsupported,
			"image digest references are not supported: %s",
			ref,
		)
	}

	colon := strings.LastIndex(ref, ":")

	// A colon followed by a / is a registry port, not a tag.
	if colon == -1 || strings.Contains(ref[colon:], "/") {
		return ref, defaultImageTag, nil
	}

	return ref[:colon], ref[colon+1:], nil
}

func resolveServiceImage(
	ctx context.Context,
	svcExtImage *ServiceImage,
	svc ComposeService,
	gitClient git.Client,
) error {
	svcImageRepo, svcImageTag, err := parseImageRef(svc.Image)
	if err != nil {
		return err
	}

	if svcExtImage.Repository == "" {
		svcExtImage.Repository = svcImageRepo
	}

	if svcExtImage.Tag == "" {
		svcExtImage.Tag = svcImageTag
	}

	if svcExtImage.Repository == "" {
		err = errors.Join(
			err,
			errs.Errorf(
				ErrImageRepositoryMissing,
				"image.repository must be specified in service extension",
			),
		)
	}

	if svcExtImage.Tag == "" {
		err = errors.Join(
			err,
			errs.Errorf(
				ErrImageTagMissing,
				"image.tag must be specified in service extension",
			),
		)
	}

	if strings.Contains(svcExtImage.Tag, "+git") {
		if err := expandGitTag(ctx, svcExtImage, gitClient); err != nil {
			return err
		}
	}

	svcExtImage.Repository = strings.TrimSpace(svcExtImage.Repository)
	svcExtImage.Tag = strings.TrimSpace(svcExtImage.Tag)

	if len(svcExtImage.Platforms) == 0 {
		svcExtImage.Platforms = []string{defaultImagePlatform}
	}

	return err
}

func resolveNgrok(
	topLevel *NgrokTopLevel,
	serviceLevel *NgrokServiceLevel,
	containerPorts []uint16,
	ngrokEnabled bool,
) error {
	if serviceLevel == nil {
		// A simple guard just in case caller passed in nil pointer
		return nil
	}

	// Zeroed rather than left alone, so a zero Port is the signal for "off".
	if !ngrokEnabled {
		serviceLevel.Port = 0
		serviceLevel.TrafficPolicy = ""
		serviceLevel.URL = ""

		return nil
	}

	if serviceLevel.TrafficPolicy == "" &&
		topLevel != nil &&
		topLevel.TrafficPolicy != "" {
		serviceLevel.TrafficPolicy = topLevel.TrafficPolicy
	}

	if serviceLevel.Port == 0 {
		return nil
	}

	hasContainerPort := slices.ContainsFunc(
		containerPorts,
		func(p uint16) bool {
			return p == serviceLevel.Port
		},
	)

	if !hasContainerPort {
		return errs.Errorf(
			ErrNgrokPortMismatch,
			"ngrok port %d matches no port for this service: expected one of %v",
			serviceLevel.Port,
			containerPorts,
		)
	}

	return nil
}
