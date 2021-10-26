CREATE TABLE IF NOT EXISTS RetentionPolicies (
	Id varchar(26) NOT NULL,
	DisplayName varchar(64) DEFAULT NULL,
	PostDuration  bigint(20) DEFAULT NULL,
    PRIMARY KEY (Id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @preparedStatement = (SELECT IF(
    (
        SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
        WHERE table_name = 'RetentionPolicies'
        AND table_schema = DATABASE()
        AND index_name = 'IDX_RetentionPolicies_DisplayName_Id'
    ) > 0,
    'DROP INDEX IDX_RetentionPolicies_DisplayName_Id ON RetentionPolicies;',
    'SELECT 1'
));

PREPARE removeIndexIfExists FROM @preparedStatement;
EXECUTE removeIndexIfExists;
DEALLOCATE PREPARE removeIndexIfExists;

SET @preparedStatement = (SELECT IF(
    (
        SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
        WHERE table_name = 'RetentionPolicies'
        AND table_schema = DATABASE()
        AND index_name = 'IDX_RetentionPolicies_DisplayName'
    ) > 0,
    'SELECT 1',
    'CREATE INDEX IDX_RetentionPolicies_DisplayName ON RetentionPolicies(DisplayName);'
));

PREPARE createIndexIfNotExists FROM @preparedStatement;
EXECUTE createIndexIfNotExists;
DEALLOCATE PREPARE createIndexIfNotExists;
