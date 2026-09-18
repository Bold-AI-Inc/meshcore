/**
 * Content-block builders -- match the wire contract exactly
 * (server/internal/gateway/contract.go's contentBlockRequest): each block
 * is { type, text?, media_type?, data?, url? }.
 *
 * Every builder besides textBlock() accepts path/data/mediaType/url as
 * either a single value or an array. Passing an array on any one of them
 * builds several blocks at once and returns an array, e.g.:
 *
 *   imageBlock({ path: ["photo.jpg", "ninja.jpg"] })
 *   -> [ {...block for photo.jpg...}, {...block for ninja.jpg...} ]
 *
 * so it can be spliced straight into a content: [...] array -- llmCaller
 * flattens one level of nested arrays in `content` before sending, so
 * mixing scalar and batch blocks in the same call just works:
 *
 *   content: [
 *     textBlock("Compare these"),
 *     imageBlock({ path: ["photo.jpg", "ninja.jpg"] }),
 *   ]
 */

import type { ContentBlock, ContentBlockType } from "./types";

export function textBlock(text: string): ContentBlock {
  return { type: "text", text };
}

function asArray<T>(value: T | T[] | undefined): T[] | undefined {
  if (value === undefined) return undefined;
  return Array.isArray(value) ? value : [value];
}

function base64Encode(bytes: Uint8Array): string {
  if (typeof Buffer !== "undefined") {
    return Buffer.from(bytes).toString("base64"); // Node
  }
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary); // browser
}

function guessMediaType(path: string): string | undefined {
  const ext = path.slice(path.lastIndexOf(".") + 1).toLowerCase();
  const table: Record<string, string> = {
    jpg: "image/jpeg", jpeg: "image/jpeg", png: "image/png", gif: "image/gif",
    webp: "image/webp", bmp: "image/bmp", svg: "image/svg+xml", heic: "image/heic",
    pdf: "application/pdf", doc: "application/msword", txt: "text/plain",
    md: "text/markdown", csv: "text/csv", json: "application/json",
    docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
    mp3: "audio/mpeg", wav: "audio/wav", ogg: "audio/ogg", m4a: "audio/mp4",
    flac: "audio/flac", aac: "audio/aac",
    mp4: "video/mp4", mov: "video/quicktime", webm: "video/webm", mpeg: "video/mpeg",
  };
  return table[ext];
}

async function readFile(path: string): Promise<Uint8Array> {
  const fs = await import("fs/promises");
  return new Uint8Array(await fs.readFile(path));
}

interface BlockOpts {
  path?: string | string[];
  data?: Uint8Array | Uint8Array[];
  mediaType?: string | string[];
  url?: string | string[];
}

async function oneBlock(
  type: ContentBlockType,
  path: string | undefined,
  data: Uint8Array | undefined,
  mediaType: string | undefined,
  url: string | undefined,
  defaultMediaType?: string,
): Promise<ContentBlock> {
  const sources = [path, data, url].filter((x) => x !== undefined).length;
  if (sources !== 1) {
    throw new Error(`${type} block needs exactly one of path, data, or url`);
  }

  if (url !== undefined) {
    return { type, url };
  }

  let resolvedMediaType = mediaType;
  let resolvedData = data;
  if (path !== undefined) {
    const guessed = guessMediaType(path);
    resolvedMediaType = mediaType || guessed || defaultMediaType;
    if (!resolvedMediaType) {
      throw new Error(`could not guess a media type for ${path} -- pass mediaType explicitly`);
    }
    resolvedData = await readFile(path);
  } else if (!resolvedMediaType) {
    resolvedMediaType = defaultMediaType;
  }

  if (!resolvedMediaType) {
    throw new Error(`${type} block with inline data needs mediaType`);
  }

  return { type, media_type: resolvedMediaType, data: base64Encode(resolvedData!) };
}

async function buildBlocks(
  type: ContentBlockType,
  opts: BlockOpts,
  defaultMediaType?: string,
): Promise<ContentBlock | ContentBlock[]> {
  const isBatch = Array.isArray(opts.path) || Array.isArray(opts.data) || Array.isArray(opts.url);

  const paths = asArray(opts.path);
  const datas = asArray(opts.data);
  const urls = asArray(opts.url);
  const mediaTypes = Array.isArray(opts.mediaType) ? opts.mediaType : undefined;

  const n = (paths ?? datas ?? urls ?? [undefined]).length;
  for (const [name, seq] of [["path", paths], ["data", datas], ["url", urls], ["mediaType", mediaTypes]] as const) {
    if (seq !== undefined && seq.length !== n) {
      throw new Error(`${type} block: ${name} array length (${seq.length}) doesn't match the others (${n})`);
    }
  }

  const blocks = await Promise.all(
    Array.from({ length: n }, (_, i) =>
      oneBlock(
        type,
        paths?.[i],
        datas?.[i],
        mediaTypes ? mediaTypes[i] : (opts.mediaType as string | undefined),
        urls?.[i],
        defaultMediaType,
      ),
    ),
  );
  return isBatch ? blocks : blocks[0];
}

/** An image content block (or array of blocks, if any option is an array). */
export function imageBlock(opts: BlockOpts): Promise<ContentBlock | ContentBlock[]> {
  return buildBlocks("image", opts);
}

/** A document content block (falls back to application/pdf only when the
 * extension can't be identified and no mediaType was given). */
export function documentBlock(opts: BlockOpts): Promise<ContentBlock | ContentBlock[]> {
  return buildBlocks("document", opts, "application/pdf");
}

export function audioBlock(opts: BlockOpts): Promise<ContentBlock | ContentBlock[]> {
  return buildBlocks("audio", opts);
}

export function videoBlock(opts: BlockOpts): Promise<ContentBlock | ContentBlock[]> {
  return buildBlocks("video", opts);
}

/** Flattens one level of nesting -- lets a batch block builder's array
 * output sit directly inside a content: [...] array alongside scalar
 * blocks. */
export function flattenContent(content: (ContentBlock | ContentBlock[])[]): ContentBlock[] {
  const flat: ContentBlock[] = [];
  for (const item of content) {
    if (Array.isArray(item)) flat.push(...item);
    else flat.push(item);
  }
  return flat;
}
