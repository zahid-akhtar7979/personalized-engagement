-- =====================================================
-- STAGING TABLES
-- =====================================================

CREATE TABLE IF NOT EXISTS stg_events (
    timestamp BIGINT,
    visitorid BIGINT,
    event VARCHAR(50),
    itemid BIGINT,
    transactionid VARCHAR(100)
);

CREATE TABLE IF NOT EXISTS stg_category_tree (
    categoryid BIGINT,
    parentid BIGINT
);

CREATE TABLE IF NOT EXISTS stg_item_properties (
    timestamp BIGINT,
    itemid BIGINT,
    property VARCHAR(255),
    value TEXT
);

-- =====================================================
-- LOAD CSV FILES (Retailrocket dataset)
-- =====================================================

COPY stg_events
FROM '/import-data/events.csv'
DELIMITER ','
CSV HEADER;

COPY stg_category_tree
FROM '/import-data/category_tree.csv'
DELIMITER ','
CSV HEADER;

COPY stg_item_properties
FROM '/import-data/item_properties_part1.csv'
DELIMITER ','
CSV HEADER;

COPY stg_item_properties
FROM '/import-data/item_properties_part2.csv'
DELIMITER ','
CSV HEADER;

-- =====================================================
-- LOAD CATEGORIES
-- =====================================================

INSERT INTO categories(id, name, parent_id)
SELECT
    categoryid,
    'Category-' || categoryid::TEXT,
    NULLIF(parentid, -1)
FROM stg_category_tree
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- LOAD USERS (visitorid = user id for event consistency)
-- =====================================================

INSERT INTO users(id, username, email)
SELECT DISTINCT
    visitorid,
    'user_' || visitorid,
    'user_' || visitorid || '@demo.com'
FROM stg_events
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- LOAD CONTENT CATALOG
-- =====================================================

INSERT INTO content_catalog(item_id, title, category_id, category, price)
SELECT
    ip.itemid,
    COALESCE(
        MAX(ip.value) FILTER (WHERE ip.property = 'title'),
        'Product-' || ip.itemid::TEXT
    ),
    MAX(ip.value::BIGINT) FILTER (WHERE ip.property = 'categoryid'),
    'Category-' || MAX(ip.value) FILTER (WHERE ip.property = 'categoryid'),
    COALESCE(
        NULLIF(MAX(ip.value) FILTER (WHERE ip.property = 'price'), '')::DECIMAL,
        0
    )
FROM stg_item_properties ip
GROUP BY ip.itemid
HAVING MAX(ip.value) FILTER (WHERE ip.property = 'categoryid') IS NOT NULL
ON CONFLICT (item_id) DO NOTHING;

-- =====================================================
-- LOAD USER EVENTS (map Retailrocket events to platform types)
-- =====================================================

INSERT INTO user_events(event_id, user_id, item_id, category_id, event_type, timestamp)
SELECT
    md5(
        e.visitorid::TEXT ||
        COALESCE(e.itemid::TEXT, '0') ||
        e.timestamp::TEXT ||
        e.event
    ),
    e.visitorid,
    e.itemid,
    cat.category_id,
    CASE e.event
        WHEN 'view' THEN 'VIEWED'
        WHEN 'addtocart' THEN 'ADD_TO_CART'
        WHEN 'transaction' THEN 'PURCHASED'
        ELSE UPPER(e.event)
    END,
    to_timestamp(e.timestamp / 1000.0)
FROM stg_events e
LEFT JOIN (
    SELECT
        itemid,
        MAX(value::BIGINT) AS category_id
    FROM stg_item_properties
    WHERE property = 'categoryid'
    GROUP BY itemid
) cat ON cat.itemid = e.itemid
ON CONFLICT (event_id) DO NOTHING;

-- =====================================================
-- SAMPLE RECOMMENDATIONS (from real event data)
-- =====================================================

INSERT INTO recommendations(user_id, item_id, score, reason, section)
SELECT
    visitorid,
    itemid,
    0.85,
    'Popular among similar users',
    'recommendedForYou'
FROM (
    SELECT DISTINCT visitorid, itemid
    FROM stg_events
    WHERE itemid IS NOT NULL
    LIMIT 10000
) t;

-- =====================================================
-- INITIAL ANALYTICS SNAPSHOT (from real event counts)
-- =====================================================

INSERT INTO analytics_metrics(
    retention_rate,
    ctr,
    conversion_rate,
    engagement_score,
    roi_percentage,
    active_users
)
SELECT
    ROUND(
        100.0 * COUNT(DISTINCT visitorid) FILTER (
            WHERE visitorid IN (
                SELECT visitorid FROM stg_events GROUP BY visitorid HAVING COUNT(*) > 1
            )
        ) / NULLIF(COUNT(DISTINCT visitorid), 0),
        2
    ),
    6.20,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE event = 'transaction')
        / NULLIF(COUNT(*) FILTER (WHERE event = 'view'), 0),
        2
    ),
    COUNT(*) FILTER (WHERE event = 'view')
        + 3 * COUNT(*) FILTER (WHERE event = 'addtocart')
        + 5 * COUNT(*) FILTER (WHERE event = 'transaction'),
    120.50,
    COUNT(DISTINCT visitorid)
FROM stg_events;

-- =====================================================
-- CLEANUP STAGING
-- =====================================================

DROP TABLE stg_events;
DROP TABLE stg_category_tree;
DROP TABLE stg_item_properties;
