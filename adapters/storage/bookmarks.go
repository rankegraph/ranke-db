// package: storage / composition
// type:    logic
// job:     CheckBookmarks — name the layers when a storage tree can hold no 𝒰_hist
// limits:  reads Capabilities; the capability is each backend's to report (-> ranke-go)
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/config/scope"
)

// ErrNoBookmarks reports a storage tree that cannot hold 𝒰_hist.
var ErrNoBookmarks = errors.New("storage: no configured layer can hold a bookmark store")

// CheckBookmarks reports whether u can hold this archive's bookmark list. Both sequencer
// backends refuse such a Universe themselves; the layer names are what they cannot say,
// and the composition rules differ, so the remedy does too.
func CheckBookmarks(ctx context.Context, sec scope.Section, u ranke.Universe) error {
	if u == nil || u.Capabilities().Bookmarks {
		return nil
	}
	layers, err := Describe(ctx, sec)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoBookmarks, err)
	}
	named := make([]string, 0, len(layers))
	for _, l := range layers {
		if l.Name == l.Type {
			named = append(named, l.Type)
			continue
		}
		named = append(named, fmt.Sprintf("%s (%s)", l.Name, l.Type))
	}

	t, err := typeOf(ctx, sec)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoBookmarks, err)
	}
	var rule string
	switch t {
	case "stack":
		rule = "a stack holds bookmarks when one of its authoritative layers does, " +
			"so give it an authoritative layer whose backend holds them"
	case "partition":
		rule = "a partition replicates the bookmark list to every shard, so it holds " +
			"bookmarks only when all of them do — replace the shard that does not"
	default:
		rule = "choose a backend that holds bookmarks"
	}

	return fmt.Errorf("%w: configured layers are %s; %s. A backend that is a rebuildable "+
		"projection reports none, the head being lost with the reindex",
		ErrNoBookmarks, strings.Join(named, ", "), rule)
}
