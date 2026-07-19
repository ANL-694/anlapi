-- Existing Grok groups predate the shared media-generation capability gate.
UPDATE groups
SET allow_image_generation = true
WHERE platform = 'grok'
  AND allow_image_generation = false;
