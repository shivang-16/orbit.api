DELETE FROM model_catalogue
WHERE model_id IN (
    'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1',
    'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.openai.gpt-6-astra'
);
