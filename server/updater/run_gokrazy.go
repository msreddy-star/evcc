package updater

import (
	"fmt"
	"net/http"

	"github.com/evcc-io/evcc/util"
	"github.com/google/go-github/v32/github"
)

var latest *github.RepositoryRelease

// Run regularly checks for new GitHub releases and updates the available version info
func Run(log *util.Logger, httpd webServer, outChan chan<- util.Param) {
	u := &watch{
		log:     log,
		outChan: outChan,
		repo:    NewRepo(log, owner, repository),
	}

	// Register HTTP endpoint for manual update trigger
	httpd.Router().PathPrefix("/api/update").HandlerFunc(u.updateHandler)

	// Channel to receive new releases
	c := make(chan *github.RepositoryRelease, 1)

	// Continuously watch for new GitHub releases
	go u.watchReleases(util.Version, c)

	// Signal that update capability is available
	u.Send("hasUpdater", true)

	// Listen for updates
	for rel := range c {
		latest = rel
		u.Send("availableVersion", *latest.TagName)
	}
}

const rootFSAsset = "evcc_%s.rootfs.gz"

// updateHandler triggers the update process using the latest GitHub release
func (u *watch) updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	if latest == nil {
		http.Error(w, "No release available for update", http.StatusBadRequest)
		return
	}

	// Construct the asset name for the root filesystem image
	name := fmt.Sprintf(rootFSAsset, *latest.TagName)

	// Look for the corresponding release asset on GitHub
	assetID, size, err := u.repo.FindReleaseAsset(name)
	if err != nil {
		http.Error(w, fmt.Sprintf("RootFS image not found: %v", err), http.StatusBadRequest)
		return
	}

	// Attempt to perform the update
	if err := u.execute(assetID, size); err != nil {
		u.log.ERROR.Printf("Update failed: %v", err)
		http.Error(w, fmt.Sprintf("Update failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Update triggered successfully")
}
