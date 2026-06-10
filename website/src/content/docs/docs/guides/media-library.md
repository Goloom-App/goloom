---
title: Media library
description: Upload, browse, and reuse images and videos across posts.
---

The **Media library** stores visual assets for your workspace. Uploaded files can be attached in the [Composer](/docs/guides/composer/) and referenced by automation templates.

## Asset grid

All uploads appear as a thumbnail grid. Click an item to inspect details or use it when composing a post.

Supported types depend on your connected providers; goloom validates attachments before scheduling.

## Metadata

Selecting a file opens a sidebar with:

- **File size** and **dimensions**
- **Upload date**
- **Used in N posts** — how often the asset appears in scheduled or published content

Use the usage counter to retire assets that are no longer needed or to find posts that share the same image.

## Storage layout

On disk, media is stored under `data/media/{team_id}/{hash}`. When migrating to Kubernetes or another host, copy this tree along with your database — see [Docker to Kubernetes](/docs/migrations/docker-to-kubernetes/).

## Tips

- Add **alt text** in the composer, not only in the library, so each post can describe the image in context.
- Prefer the library over re-uploading the same file — deduplication keeps storage predictable.

## Related guides

- [Composer](/docs/guides/composer/) — attach media to posts
- [Teams](/docs/guides/teams/) — media is scoped per workspace
