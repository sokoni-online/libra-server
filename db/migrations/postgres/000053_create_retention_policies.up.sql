CREATE TABLE IF NOT EXISTS retentionpolicies (
	id VARCHAR(26),
	displayname VARCHAR(64),
	postduration bigint,
    PRIMARY KEY (id)
);

DROP INDEX IF EXISTS IDX_RetentionPolicies_DisplayName_Id;
CREATE INDEX IF NOT EXISTS IDX_RetentionPolicies_DisplayName ON retentionpolicies (displayname);
