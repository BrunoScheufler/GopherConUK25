package store

import (
	"context"
	"fmt"
)

// GetTopAccountsByNotes returns the top n accounts ordered by note count (descending)
func GetTopAccountsByNotes(ctx context.Context, accountStore AccountStore, noteStore NoteStore, limit int) ([]AccountStats, error) {
	accounts, err := accountStore.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var accountStats []AccountStats
	for _, account := range accounts {
		noteCount, err := noteStore.CountNotes(ctx, account.ID)
		if err != nil {
			// Log error but continue with other accounts
			continue
		}
		
		accountStats = append(accountStats, AccountStats{
			Account:   account,
			NoteCount: noteCount,
		})
	}

	// Sort by note count (descending)
	for i := 0; i < len(accountStats)-1; i++ {
		for j := 0; j < len(accountStats)-i-1; j++ {
			if accountStats[j].NoteCount < accountStats[j+1].NoteCount {
				accountStats[j], accountStats[j+1] = accountStats[j+1], accountStats[j]
			}
		}
	}

	// Return top n accounts
	if limit > 0 && limit < len(accountStats) {
		accountStats = accountStats[:limit]
	}

	return accountStats, nil
}