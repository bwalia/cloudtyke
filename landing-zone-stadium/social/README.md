# Sharing CloudTyke Arena

- `linkedin-post.md`: the launch post ("A stadium is built once.").
- `../site/img/og-arena.png`: the 1200×627 preview image. `site/index.html` points `og:image` at it, so LinkedIn and other sites show it when the Arena link is shared.
- `thumbnail-2x.png`: the same image at 2400×1254.
- `thumbnail.html`: the source of the image. To re-render it, open it in a browser at 1200×627, or use headless Chrome with `--force-device-scale-factor=2`. It uses the Arena's stylesheet and fonts.
