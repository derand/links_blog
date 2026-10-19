package store

import "context"

// LinkURL is a link's URL and dedup key.
type LinkURL struct {
	ID      int64
	URL     string
	URLNorm string
}

// LinkURLs returns the URLs of all links, hidden included, by id.
func (s *Store) LinkURLs(ctx context.Context) ([]LinkURL, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, url, url_norm FROM links ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var us []LinkURL
	for rows.Next() {
		var u LinkURL
		if err := rows.Scan(&u.ID, &u.URL, &u.URLNorm); err != nil {
			return nil, err
		}
		us = append(us, u)
	}
	return us, rows.Err()
}

// SetLinkURLs replaces the URLs and dedup keys of the given links in one
// transaction, leaving everything else, metadata included, as it is. Keys may
// move between the links; afterwards they must still be unique.
func (s *Store) SetLinkURLs(ctx context.Context, us []LinkURL) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Free the keys first, so that one link may take a key another one gives up.
	for _, u := range us {
		if _, err := tx.ExecContext(ctx, `UPDATE links SET url_norm = 'renorm:' || id WHERE id = ?`, u.ID); err != nil {
			return err
		}
	}
	for _, u := range us {
		if _, err := tx.ExecContext(ctx, `UPDATE links SET url = ?, url_norm = ? WHERE id = ?`, u.URL, u.URLNorm, u.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
