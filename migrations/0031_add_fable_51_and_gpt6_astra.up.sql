-- Add Claude Fable 5.1 and GPT-6 Astra. Idempotent: keeps existing UUIDs
-- on re-run. Does not touch any other catalogue or pricing rows.
-- Global CRIS Standard (≤272K) list prices; 1M context window.
-- Claude Fable 5.1: $10 / $50 per 1M tokens.
-- GPT-6 Astra:     $11 / $55 per 1M tokens (In-Region / Geo CRIS; Global CRIS is $10 / $50).
-- https://aws.amazon.com/bedrock/pricing/
-- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-openai-gpt-6-astra.html

INSERT INTO model_catalogue (
    name, slug, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active, model_released_date
) VALUES
    (
        'Claude Fable 5.1',
        'claude-fable-5-1',
        'anthropic',
        'bedrock',
        'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1',
        1000000,
        0,
        ARRAY['flagship','long-context','vision','reasoning','coding','agentic']::text[],
        ARRAY['text','image']::text[],
        true,
        '2026-09-01'
    ),
    (
        'GPT 6 Astra',
        'gpt-6-astra',
        'openai',
        'bedrock',
        'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.openai.gpt-6-astra',
        1000000,
        0,
        ARRAY['flagship','long-context','vision','reasoning','coding','agentic']::text[],
        ARRAY['text','image']::text[],
        true,
        '2026-09-08'
    )
ON CONFLICT (model_id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    vendor = EXCLUDED.vendor,
    provider = EXCLUDED.provider,
    input_context_limit = EXCLUDED.input_context_limit,
    sort_order = EXCLUDED.sort_order,
    tags = EXCLUDED.tags,
    modalities = EXCLUDED.modalities,
    is_active = EXCLUDED.is_active,
    model_released_date = EXCLUDED.model_released_date;

INSERT INTO model_pricing (
    model_catalogue_id,
    vendor_input_per_million_micros,
    vendor_output_per_million_micros
)
SELECT c.id, p.input_micros, p.output_micros
FROM (
    VALUES
        ('arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1', 10000000::bigint, 50000000::bigint),
        ('arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.openai.gpt-6-astra', 11000000, 55000000)
) AS p(model_id, input_micros, output_micros)
JOIN model_catalogue c ON c.model_id = p.model_id
ON CONFLICT (model_catalogue_id) DO UPDATE SET
    vendor_input_per_million_micros = EXCLUDED.vendor_input_per_million_micros,
    vendor_output_per_million_micros = EXCLUDED.vendor_output_per_million_micros;
