# File Attachments and PDF Handling

This document describes how rune should resolve local file references in user prompts, attach images/PDFs, and handle provider/model capability differences.

## Goals

- If the user references a local file path in a prompt, assume they want rune to use that file as context.
- Support both explicit `@file` mentions and plain inline file paths.
- Attach **all** valid referenced PDFs and images, not just the first one.
- Prefer native provider file/image inputs when the selected provider/model supports them.
- Fall back to local PDF text extraction when native PDF/document input is unsupported.
- Never silently drop files. Show clear warnings/errors for missing, invalid, too-large, unsupported, or unreadable files.

## User-facing behavior

### Inline local paths

If a prompt contains a path to an existing local file, rune should resolve and attach it.

Examples:

```text
summarize paper.pdf
compare ./docs/a.pdf ../docs/b.pdf
analyze screenshot.png report.pdf
```

If the referenced files exist, rune should attach all valid files before sending the request to the model.

### `@` mentions

`@` remains supported as an explicit attachment/reference syntax.

Examples:

```text
summarize @paper.pdf
compare @./docs/a.pdf @"../docs/report final.pdf"
analyze @screenshot.png
```

`@` is especially useful for paths with spaces or punctuation.

### Multiple files

If the user prompt references multiple PDFs/images, rune should attach **all** valid referenced files.

Example:

```text
Compare paper1.pdf paper2.pdf and these screenshots: ./a.png ./b.png
```

Expected attachments:

- `paper1.pdf`
- `paper2.pdf`
- `a.png`
- `b.png`

Invalid files should not prevent valid files from being attached.

Example:

```text
summarize a.pdf missing.pdf b.pdf
```

If `a.pdf` and `b.pdf` exist but `missing.pdf` does not, rune should attach `a.pdf` and `b.pdf`, and warn about `missing.pdf`.

### Duplicates

If the same file appears multiple times, attach it once and preserve the original prompt text.

Example:

```text
compare a.pdf with @a.pdf
```

Expected behavior:

- Attach `a.pdf` once.
- Keep enough text context for the model to know the user referenced `a.pdf`.

## Supported file categories

### Text files

Text files can continue to be inlined as text blocks, using the existing `<file>` style.

Example internal representation:

```xml
<file name="/abs/path/README.md">
...
</file>
```

### Images

Images should be validated and attached as image content blocks.

Supported image extensions should include at least:

- `.png`
- `.jpg`
- `.jpeg`
- `.gif`
- `.webp`
- `.bmp`
- `.tiff`
- `.tif`
- `.ico`
- `.heic`
- `.heif`

Provider/model support must be checked before sending image inputs.

### PDFs/documents

PDFs should be validated and represented as document/file attachment blocks.

Recommended internal block:

```go
type DocumentBlock struct {
    Data     []byte `json:"data,omitempty"`
    Text     string `json:"text,omitempty"`
    MimeType string `json:"mime_type"`
    Name     string `json:"name,omitempty"`
    Path     string `json:"path,omitempty"`
}
```

For PDFs:

```text
MimeType = application/pdf
```

Provider request builders can then choose between:

- Native PDF/document attachment
- Extracted text fallback
- Clear warning/error when neither is possible

## Provider/model capability matrix

| Provider/model family | Images | Native PDFs/documents | rune behavior |
|---|---:|---:|---|
| Codex/OpenAI Responses API | Yes for vision-capable models | Yes via `input_file` | Send images natively when supported. Send PDFs as native `input_file` where possible. |
| Anthropic Claude | Yes when `ImageInput` capability is true | Yes when `PDFInput` capability is true | Query/use model capabilities where possible. Fallback to extracted PDF text if `PDFInput` is false. |
| Gemini | Yes for multimodal models | Yes via `application/pdf` inline data or Files API | Send PDFs natively for document-capable models. Use Files API for large/reused PDFs. |
| Groq | Yes only for vision-capable models | No confirmed native PDF chat/document input | Send images only for known vision models. Fallback to PDF text extraction. |
| Ollama | Yes only for local vision models | No native PDF/document attachment found | Send images only for local vision models. Fallback to PDF text extraction. |

## Native attachment vs fallback policy

For each resolved attachment:

1. If the provider/model supports native input for that file type, send it natively.
2. If the file is a PDF/document and native input is unsupported, extract text locally and send the extracted text.
3. If no safe fallback exists, warn clearly and do not silently drop the file.

Pseudo-policy:

```go
for each attachment:
    if providerSupportsNativeAttachment(provider, model, attachment.MimeType):
        sendNative(attachment)
    else if attachment.MimeType == "application/pdf":
        text, ok := extractPDFText(attachment.Path)
        if ok:
            sendTextDocument(attachment, text)
        else:
            warnUser("could not extract text from PDF")
    else:
        warnUser("attachment type unsupported by selected provider/model")
```

## Provider-specific notes

### Codex/OpenAI

OpenAI Responses API supports files as `input_file` items. A PDF can be sent using base64 data, a Files API `file_id`, or an external URL.

Example request content:

```json
{
  "type": "input_file",
  "filename": "paper.pdf",
  "file_data": "data:application/pdf;base64,..."
}
```

For rune's Codex/OpenAI provider:

- Images should continue using native image content where model support is known.
- PDFs should use native `input_file` when supported.
- If OpenAI rejects a model/file combination, rune can fallback to extracted text when possible.

### Anthropic Claude

Anthropic exposes model capabilities through its Models API, including:

- `ImageInput`
- `PDFInput`
- `Citations`

Anthropic document blocks support base64 data, plain text, URLs, and file IDs. PDF citations can reference page locations.

Recommended behavior:

```text
if PDFInput supported:
    send native PDF document block
else:
    extract PDF text locally and send text
```

### Gemini

Gemini supports PDF input via `application/pdf` inline data and uploaded files through the Files API.

Recommended behavior:

```text
small PDF -> inlineData application/pdf
large/reused PDF -> Files API upload
unsupported/unknown model -> extracted text fallback
```

Gemini docs also describe processing multiple PDFs.

### Groq

Groq supports image input for vision-capable models, including examples using models such as:

```text
meta-llama/llama-4-scout-17b-16e-instruct
```

No official native PDF/document chat input support was confirmed. Groq PDFs should therefore use text extraction fallback.

### Ollama

Ollama supports images for local vision models. The selected model must support vision.

No native PDF/document input support was confirmed. Ollama PDFs should therefore use text extraction fallback.

Future optional feature: render PDF pages to images and send them to a vision model. This should be separate because it can be expensive and token-heavy.

## PDF validation

Before attaching or extracting a PDF, validate at least:

- Path exists.
- Path points to a regular file.
- File is readable.
- File size is below configured limit.
- Extension is `.pdf` and/or MIME sniffing identifies PDF.
- File starts with `%PDF-`.

Optional stronger validation:

- Parse page count.
- Detect encrypted/password-protected PDFs.
- Reject malformed PDFs.
- Enforce extraction timeout.

Recommended configurable limits:

```toml
[attachments]
max_files = 20
max_total_bytes = 50000000

[pdf]
max_bytes = 25000000
max_pages = 100
max_extracted_chars = 200000
```

Exact defaults can be tuned later.

## PDF text extraction fallback

When native PDF input is unsupported, extract PDF text locally and send it in a document block or text block.

Example fallback prompt content:

```xml
<document name="/abs/path/paper.pdf" type="application/pdf" source="extracted-text">
...
</document>
```

If the extracted text is truncated:

```xml
<document name="/abs/path/paper.pdf" type="application/pdf" source="extracted-text" truncated="true">
...
</document>
```

If the PDF has no extractable text, warn the user:

```text
(could not extract text from paper.pdf: PDF may be scanned or image-only; use a native PDF-capable model or OCR first)
```

Potential extraction backends:

- Pure Go library for portability.
- Optional `pdftotext`/Poppler when available.
- Optional OCR integration later, e.g. OCRmyPDF/Tesseract, as a separate feature.

## UX messages

rune should show clear attachment status.

Examples:

```text
(attached: paper.pdf, screenshot.png)
```

```text
(attached PDFs: a.pdf, b.pdf; using native provider PDF input)
```

```text
(attached PDFs: a.pdf, b.pdf; extracted text because this provider/model does not support native PDF input)
```

```text
(could not attach missing.pdf: file not found)
```

```text
(could not attach huge.pdf: file size exceeds 25 MB limit)
```

```text
(could not extract text from scan.pdf: no extractable text found)
```

Do not silently ignore failures.

## Implementation plan

### Phase 1: unified file reference resolution

- Create a resolver that scans user prompt text for:
  - `@file` mentions
  - quoted `@"file with spaces.pdf"` mentions
  - plain inline local paths
- Resolve all valid paths relative to cwd.
- Deduplicate by canonical/absolute path.
- Return:
  - original or lightly annotated text
  - attachment blocks
  - warnings

Suggested shape:

```go
type ResolvedUserInput struct {
    Text        string
    Attachments []ai.ContentBlock
    Warnings    []string
}
```

### Phase 2: content block types

- Add `ai.DocumentBlock` or a generic `ai.FileBlock`.
- Update JSON marshal/unmarshal for sessions.
- Keep `ai.ImageBlock` for images.
- Continue inlining readable text files.

### Phase 3: provider capability checks

- Extend provider capability logic with:
  - image support
  - PDF/document support
  - unknown support
- For Anthropic, use model capability metadata where available.
- For current rune providers, maintain capability tables:
  - Codex/OpenAI: native PDFs through Responses `input_file`
  - Groq: images for known vision models, PDFs fallback
  - Ollama: images for local vision models, PDFs fallback

### Phase 4: provider request serialization

- Codex/OpenAI:
  - Serialize PDFs as `input_file`.
  - Serialize images as existing `input_image`.
- Groq:
  - Serialize images only for vision-capable models.
  - Convert PDFs to extracted text.
- Ollama:
  - Serialize images only for vision-capable models.
  - Convert PDFs to extracted text.

### Phase 5: PDF extraction

- Implement PDF text extraction with limits.
- Add warnings for scanned/encrypted/oversized PDFs.
- Add tests for:
  - multiple PDFs
  - PDF + image mix
  - duplicate paths
  - missing paths
  - unsupported provider fallback
  - extraction truncation

## Open questions

- Should inline path resolution apply to all file types or only recognized attachment/document types?
  - Current recommendation: attach recognized images and PDFs; inline readable text files.
- Should absolute paths outside cwd/repo require confirmation?
  - Current recommendation: resolve if the user typed the path and it exists, but always show visible attachment status.
- Should native PDF upload be opt-out for privacy-sensitive users?
  - Possible config:

```toml
[pdf]
mode = "auto" # auto | native | text
```

- Should rune cache extracted PDF text by path/mtime/size?
  - Useful for large PDFs and repeated prompts.

## Sources

- OpenAI file inputs: https://developers.openai.com/api/docs/guides/file-inputs
- Anthropic Files API: https://platform.claude.com/docs/en/build-with-claude/files
- Anthropic model overview/capabilities: https://platform.claude.com/docs/en/about-claude/models/overview
- Gemini file input methods: https://ai.google.dev/gemini-api/docs/file-input-methods
- Gemini document processing: https://ai.google.dev/gemini-api/docs/document-processing
- Groq vision docs: https://console.groq.com/docs/vision
- Groq Responses API docs: https://console.groq.com/docs/responses-api
- Ollama vision docs: https://docs.ollama.com/capabilities/vision
- Ollama API docs: https://github.com/ollama/ollama/blob/main/docs/api.md
