-- Add Claude Fable 5.1 and GPT-6 Astra. Idempotent: matched by slug so
-- existing catalogue UUIDs and pricing rows stay intact if the Bedrock
-- model_id is rewritten (ap-south-1 ARNs are invalid on us-east-1).
-- Claude Fable 5.1: $10 / $50 per 1M tokens.
-- GPT-6 Astra:     $11 / $55 per 1M tokens (In-Region / Geo CRIS).
-- https://aws.amazon.com/bedrock/pricing/
-- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-openai-gpt-6-astra.html
-- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-1.html

INSERT INTO model_catalogue (
    name, slug, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active, model_released_date
) VALUES
    (
        'Claude Fable 5.1',
        'claude-fable-5-1',
        'anthropic',
        'bedrock',
        'arn:aws:bedrock:us-east-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1',
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
        'arn:aws:bedrock:us-east-1:471112741644:inference-profile/global.openai.gpt-6-astra',
        1000000,
        0,
        ARRAY['flagship','long-context','vision','reasoning','coding','agentic']::text[],
        ARRAY['text','image']::text[],
        true,
        '2026-09-08'
    )
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    vendor = EXCLUDED.vendor,
    provider = EXCLUDED.provider,
    model_id = EXCLUDED.model_id,
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
        ('claude-fable-5-1', 10000000::bigint, 50000000::bigint),
        ('gpt-6-astra', 11000000, 55000000)
) AS p(slug, input_micros, output_micros)
JOIN model_catalogue c ON c.slug = p.slug
ON CONFLICT (model_catalogue_id) DO UPDATE SET
    vendor_input_per_million_micros = EXCLUDED.vendor_input_per_million_micros,
    vendor_output_per_million_micros = EXCLUDED.vendor_output_per_million_micros;
