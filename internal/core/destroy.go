package core

func Destroy(flags *ComposeFlags, dryRun bool) error {
	project, err := loadComposeProject(flags)
	if err != nil {
		return err
	}

	ext, err := loadMainExtensionConfig(project)
	if err != nil {
		return err
	}

	if ext.K8s != nil {
		return k8sServicesDestroy(project, ext, dryRun)
	}

	return nil
}
