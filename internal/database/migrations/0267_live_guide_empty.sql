-- A guide downloaded without a programme in the window it keeps, while
-- the guide held some, fails with empty: the last guide with programmes
-- stays (see library.fetchGuide).
ALTER TABLE live_guides
    DROP CONSTRAINT live_guides_error_check,
    ADD CONSTRAINT live_guides_error_check
        CHECK (error IN ('', 'unreachable', 'private_network', 'too_large', 'malformed', 'channels_unreachable', 'rate_limited',
            'file_unreadable', 'file_refused', 'empty'));
