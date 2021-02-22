// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"fmt"

	"github.com/mattermost/gorp"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

type SqlPreferenceStore struct {
	*SqlStore
}

func newSqlPreferenceStore(sqlStore *SqlStore) store.PreferenceStore {
	s := &SqlPreferenceStore{sqlStore}

	for _, db := range sqlStore.GetAllConns() {
		table := db.AddTableWithName(model.Preference{}, "Preferences").SetKeys(false, "UserId", "Category", "Name")
		table.ColMap("UserId").SetMaxSize(26)
		table.ColMap("Category").SetMaxSize(32)
		table.ColMap("Name").SetMaxSize(32)
		table.ColMap("Value").SetMaxSize(2000)
	}

	return s
}

func (s SqlPreferenceStore) createIndexesIfNotExists() {
	s.CreateIndexIfNotExists("idx_preferences_user_id", "Preferences", "UserId")
	s.CreateIndexIfNotExists("idx_preferences_category", "Preferences", "Category")
	s.CreateIndexIfNotExists("idx_preferences_name", "Preferences", "Name")
}

func (s SqlPreferenceStore) deleteUnusedFeatures() {
	mlog.Debug("Deleting any unused pre-release features")

	sql := `DELETE
		FROM Preferences
	WHERE
	Category = :Category
	AND Value = :Value
	AND Name LIKE '` + store.FeatureTogglePrefix + `%'`

	queryParams := map[string]string{
		"Category": model.PREFERENCE_CATEGORY_ADVANCED_SETTINGS,
		"Value":    "false",
	}
	_, err := s.GetMaster().Exec(sql, queryParams)
	if err != nil {
		mlog.Warn("Failed to delete unused features", mlog.Err(err))
	}
}

func (s SqlPreferenceStore) Save(preferences *model.Preferences) error {
	// wrap in a transaction so that if one fails, everything fails
	transaction, err := s.GetMaster().Begin()
	if err != nil {
		return errors.Wrap(err, "begin_transaction")
	}

	defer finalizeTransaction(transaction)
	for _, preference := range *preferences {
		preference := preference
		if upsertErr := s.save(transaction, &preference); upsertErr != nil {
			return upsertErr
		}
	}

	if err := transaction.Commit(); err != nil {
		// don't need to rollback here since the transaction is already closed
		return errors.Wrap(err, "commit_transaction")
	}
	return nil
}

func (s SqlPreferenceStore) save(transaction *gorp.Transaction, preference *model.Preference) error {
	preference.PreUpdate()

	if err := preference.IsValid(); err != nil {
		return err
	}

	params := map[string]interface{}{
		"UserId":   preference.UserId,
		"Category": preference.Category,
		"Name":     preference.Name,
		"Value":    preference.Value,
	}

	if s.DriverName() == model.DATABASE_DRIVER_MYSQL {
		if _, err := transaction.Exec(
			`INSERT INTO
				Preferences
				(UserId, Category, Name, Value)
			VALUES
				(:UserId, :Category, :Name, :Value)
			ON DUPLICATE KEY UPDATE
				Value = :Value`, params); err != nil {
			return errors.Wrap(err, "failed to save Preference")
		}
		return nil
	} else if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		// postgres has no way to upsert values until version 9.5 and trying inserting and then updating causes transactions to abort
		count, err := transaction.SelectInt(
			`SELECT
				count(0)
			FROM
				Preferences
			WHERE
				UserId = :UserId
				AND Category = :Category
				AND Name = :Name`, params)
		if err != nil {
			return errors.Wrap(err, "failed to count Preferences")
		}

		if count == 1 {
			return s.update(transaction, preference)
		}
		return s.insert(transaction, preference)
	}
	return store.NewErrNotImplemented("failed to update preference because of missing driver")
}

func (s SqlPreferenceStore) insert(transaction *gorp.Transaction, preference *model.Preference) error {
	if err := transaction.Insert(preference); err != nil {
		if IsUniqueConstraintError(err, []string{"UserId", "preferences_pkey"}) {
			return store.NewErrInvalidInput("Preference", "<userId, category, name>", fmt.Sprintf("<%s, %s, %s>", preference.UserId, preference.Category, preference.Name))
		}
		return errors.Wrapf(err, "failed to save Preference with userId=%s, category=%s, name=%s", preference.UserId, preference.Category, preference.Name)
	}

	return nil
}

func (s SqlPreferenceStore) update(transaction *gorp.Transaction, preference *model.Preference) error {
	if _, err := transaction.Update(preference); err != nil {
		return errors.Wrapf(err, "failed to update Preference with userId=%s, category=%s, name=%s", preference.UserId, preference.Category, preference.Name)
	}

	return nil
}

func (s SqlPreferenceStore) Get(userId string, category string, name string) (*model.Preference, error) {
	var preference *model.Preference

	if err := s.GetReplica().SelectOne(&preference,
		`SELECT
			*
		FROM
			Preferences
		WHERE
			UserId = :UserId
			AND Category = :Category
			AND Name = :Name`, map[string]interface{}{"UserId": userId, "Category": category, "Name": name}); err != nil {
		return nil, errors.Wrapf(err, "failed to find Preference with userId=%s, category=%s, name=%s", userId, category, name)
	}
	return preference, nil
}

func (s SqlPreferenceStore) GetCategory(userId string, category string) (model.Preferences, error) {
	var preferences model.Preferences

	if _, err := s.GetReplica().Select(&preferences,
		`SELECT
				*
			FROM
				Preferences
			WHERE
				UserId = :UserId
				AND Category = :Category`, map[string]interface{}{"UserId": userId, "Category": category}); err != nil {
		return nil, errors.Wrapf(err, "failed to find Preferences with userId=%s and category=%s", userId, category)
	}

	return preferences, nil

}

func (s SqlPreferenceStore) GetAll(userId string) (model.Preferences, error) {
	var preferences model.Preferences

	if _, err := s.GetReplica().Select(&preferences,
		`SELECT
				*
			FROM
				Preferences
			WHERE
				UserId = :UserId`, map[string]interface{}{"UserId": userId}); err != nil {
		return nil, errors.Wrapf(err, "failed to find Preferences with userId=%s", userId)
	}
	return preferences, nil
}

func (s SqlPreferenceStore) PermanentDeleteByUser(userId string) error {
	query :=
		`DELETE FROM
			Preferences
		WHERE
			UserId = :UserId`

	if _, err := s.GetMaster().Exec(query, map[string]interface{}{"UserId": userId}); err != nil {
		return errors.Wrapf(err, "failed to delete Preference with userId=%s", userId)
	}

	return nil
}

func (s SqlPreferenceStore) Delete(userId, category, name string) error {
	query :=
		`DELETE FROM Preferences
		WHERE
			UserId = :UserId
			AND Category = :Category
			AND Name = :Name`

	_, err := s.GetMaster().Exec(query, map[string]interface{}{"UserId": userId, "Category": category, "Name": name})

	if err != nil {
		return errors.Wrapf(err, "failed to delete Preference with userId=%s, category=%s and name=%s", userId, category, name)
	}

	return nil
}

func (s SqlPreferenceStore) DeleteCategory(userId string, category string) error {
	_, err := s.GetMaster().Exec(
		`DELETE FROM
			Preferences
		WHERE
			UserId = :UserId
			AND Category = :Category`, map[string]interface{}{"UserId": userId, "Category": category})

	if err != nil {
		return errors.Wrapf(err, "failed to delete Preference with userId=%s and category=%s", userId, category)
	}

	return nil
}

func (s SqlPreferenceStore) DeleteCategoryAndName(category string, name string) error {
	_, err := s.GetMaster().Exec(
		`DELETE FROM
			Preferences
		WHERE
			Name = :Name
			AND Category = :Category`, map[string]interface{}{"Name": name, "Category": category})

	if err != nil {
		return errors.Wrapf(err, "failed to delete Preference with category=%s and name=%s", category, name)
	}

	return nil
}

func (s SqlPreferenceStore) PermanentDeleteFlagsBatch(endTime, limit int64) (int64, error) {
	// Granular policies override global ones
	const selectQuery = `
		SELECT Preferences.Name FROM Preferences
		LEFT JOIN Posts ON Preferences.Name = Posts.Id
		LEFT JOIN Channels ON Posts.ChannelId = Channels.Id
		LEFT JOIN Teams ON Channels.TeamId = Teams.Id
		LEFT JOIN RetentionPoliciesChannels ON Posts.ChannelId = RetentionPoliciesChannels.ChannelId
		LEFT JOIN RetentionPoliciesTeams ON Channels.TeamId = RetentionPoliciesTeams.TeamId
		WHERE Category = :Category
		      AND RetentionPoliciesChannels.ChannelId IS NULL
		      AND RetentionPoliciesTeams.TeamId IS NULL
		      AND Posts.CreateAt < :EndTime
		LIMIT :Limit`
	var query string
	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		query = `
		DELETE FROM Preferences
		WHERE Name IN (
		` + selectQuery + `
		)`
	} else {
		// MySQL does not support the LIMIT clause in a subquery with IN
		query = `
		DELETE Preferences FROM Preferences INNER JOIN (
		` + selectQuery + `
		) AS A ON Preferences.Name = A.Name`
	}

	props := map[string]interface{}{"Category": model.PREFERENCE_CATEGORY_FLAGGED_POST, "EndTime": endTime, "Limit": limit}
	sqlResult, err := s.GetMaster().Exec(query, props)
	if err != nil {
		return int64(0), errors.Wrap(err, "failed to delete Preference")
	}

	rowsAffected, err := sqlResult.RowsAffected()
	if err != nil {
		return int64(0), errors.Wrap(err, "unable to get rows affected")
	}

	return rowsAffected, nil
}

func (s *SqlPreferenceStore) PermanentDeleteFlagsBatchForRetentionPolicies(now int64, limit int64) (int64, error) {
	// Channel-specific policies override team-specific policies.
	// This will delete a preference if its corresponding post was already deleted was by
	// a retention policy.
	const selectQuery = `
		SELECT Preferences.Name FROM Preferences
		LEFT JOIN Posts ON Preferences.Name = Posts.Id
		LEFT JOIN Channels ON Posts.ChannelId = Channels.Id
		LEFT JOIN RetentionPoliciesChannels ON Posts.ChannelId = RetentionPoliciesChannels.ChannelId
		LEFT JOIN RetentionPoliciesTeams ON Channels.TeamId = RetentionPoliciesTeams.TeamId
		LEFT JOIN RetentionPolicies ON
			RetentionPoliciesChannels.PolicyId = RetentionPolicies.Id
			OR (
				RetentionPoliciesChannels.PolicyId IS NULL
				AND RetentionPoliciesTeams.PolicyId = RetentionPolicies.Id
			)
		WHERE Category = :Category AND (
			:Now - Posts.CreateAt >= RetentionPolicies.PostDuration * 24 * 60 * 60 * 1000
			OR Posts.Id IS NULL
		)
		LIMIT :Limit`
	var query string
	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		query = `
		DELETE FROM Preferences
		WHERE Name IN (
		` + selectQuery + `
		)`
	} else {
		// MySQL does not support the LIMIT clause in a subquery with IN
		query = `
		DELETE Preferences FROM Preferences INNER JOIN (
		` + selectQuery + `
		) AS A ON Preferences.Name = A.Name`
	}
	props := map[string]interface{}{"Category": model.PREFERENCE_CATEGORY_FLAGGED_POST, "Now": now, "Limit": limit}
	result, err := s.GetMaster().Exec(query, props)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete Preferences")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete Preferences")
	}
	return rowsAffected, nil
}
