package cmd

import (
	"errors"
	"fmt"
)

// registerGUIAccountJob atomically associates a newly queued job with its
// profile. Account removal checks the same manager lock before inspecting the
// job trackers, preventing a job from slipping in after the removal check.
func registerGUIAccountJob(profile *guiAccountProfile, register func()) error {
	manager := guiAccounts
	if manager == nil || profile == nil || register == nil {
		return errors.New("Apple ID account profile is unavailable")
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.profiles[profile.ID] != profile || profile.removing {
		return errors.New("Apple ID account is being removed or is no longer available")
	}
	register()
	return nil
}

// guiAccountHasActiveJobs prevents an account from being removed while any
// single download or account-scoped batch job still holds its session.
func guiAccountHasActiveJobs(accountID string) error {
	activeJobs.RLock()
	for _, job := range activeJobs.jobs {
		if job.AccountID == accountID && (job.Status == "queued" || job.Status == "purchasing" || job.Status == "downloading" || job.Status == "patching") {
			activeJobs.RUnlock()
			return fmt.Errorf("this Apple ID has active downloads; wait for them to finish before removing the account")
		}
	}
	activeJobs.RUnlock()

	batchCheckJobs.RLock()
	for _, job := range batchCheckJobs.jobs {
		if job.AccountID == accountID && job.Status == "running" {
			batchCheckJobs.RUnlock()
			return fmt.Errorf("this Apple ID has an active batch check; wait for it to finish before removing the account")
		}
	}
	batchCheckJobs.RUnlock()

	batchDownloadJobs.RLock()
	for _, job := range batchDownloadJobs.jobs {
		if job.AccountID == accountID && job.Status == "running" {
			batchDownloadJobs.RUnlock()
			return fmt.Errorf("this Apple ID has an active batch download; wait for it to finish before removing the account")
		}
	}
	batchDownloadJobs.RUnlock()

	return nil
}
