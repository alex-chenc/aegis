-- Migration 014: Backfill ai_analysis_session.conclusion from agent_executions.final_answer
-- For all sessions where conclusion IS NULL but agent_executions has a final_answer

DO $$
DECLARE
    rec RECORD;
    v_verdict TEXT;
    v_summary TEXT;
    v_reasoning TEXT;
    v_conclusion JSONB;
    v_count INTEGER := 0;
BEGIN
    FOR rec IN
        SELECT s.session_id, ae.final_answer, ae.status AS exec_status
        FROM ai_analysis_session s
        JOIN agent_executions ae ON ae.session_id = s.session_id
        WHERE s.conclusion IS NULL
          AND ae.final_answer IS NOT NULL
          AND ae.final_answer != ''
    LOOP
        v_reasoning := rec.final_answer;

        -- Keyword-based verdict detection (matches Go parseConclusionFromAnswer logic)
        v_verdict := 'unknown';
        IF rec.final_answer ILIKE '%malicious%' OR rec.final_answer ILIKE '%恶意%' OR rec.final_answer ILIKE '%threat%' THEN
            v_verdict := 'malicious';
        ELSIF rec.final_answer ILIKE '%suspicious%' OR rec.final_answer ILIKE '%可疑%' THEN
            v_verdict := 'suspicious';
        ELSIF rec.final_answer ILIKE '%benign%' OR rec.final_answer ILIKE '%false positive%' OR rec.final_answer ILIKE '%误报%' OR rec.final_answer ILIKE '%良性%' THEN
            v_verdict := 'benign';
        END IF;

        -- Build summary based on verdict
        CASE v_verdict
            WHEN 'malicious' THEN v_summary := '恶意';
            WHEN 'suspicious' THEN v_summary := '可疑';
            WHEN 'benign' THEN v_summary := '良性/误报';
            ELSE
                -- For unknown, use truncated text
                IF LENGTH(rec.final_answer) > 200 THEN
                    v_summary := SUBSTRING(rec.final_answer FROM 1 FOR 200) || '...';
                ELSE
                    v_summary := rec.final_answer;
                END IF;
        END CASE;

        -- Build conclusion JSONB
        v_conclusion := jsonb_build_object(
            'verdict', v_verdict,
            'summary', v_summary,
            'reasoning', v_reasoning
        );

        -- Update session
        UPDATE ai_analysis_session
        SET conclusion = v_conclusion,
            status = 'completed',
            concluded_at = COALESCE(concluded_at, NOW()),
            updated_at = NOW()
        WHERE session_id = rec.session_id;

        v_count := v_count + 1;
        RAISE NOTICE 'Backfilled session % with verdict=%', rec.session_id, v_verdict;
    END LOOP;

    RAISE NOTICE 'Total sessions backfilled: %', v_count;
END $$;
