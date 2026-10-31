"use client";

// VideoCard shows a YouTube video as a light preview (thumbnail + play
// button). The player (youtube-nocookie, Spanish subtitles when available)
// only loads when the learner presses play, so pages stay fast and nothing
// is sent to YouTube until then. Offline (desktop install without internet)
// the thumbnail fails and the card keeps the title and a link.

import { useState } from "react";
import { useI18n } from "@/lib/i18n";
import type { Video } from "@/lib/study";

export default function VideoCard({ video, playing, onPlay }: { video: Video; playing: boolean; onPlay: () => void }) {
  const { t, lang } = useI18n();
  const [thumb, setThumb] = useState(true);
  const src = `https://www.youtube-nocookie.com/embed/${video.id}?autoplay=1&rel=0&cc_load_policy=1&cc_lang_pref=${lang}&hl=${lang}`;
  return (
    <figure className="vd-card" id={"vid-" + video.id}>
      <div className="vd-frame">
        {playing ? (
          <iframe src={src} title={video.title} allow="autoplay; encrypted-media; picture-in-picture; fullscreen" allowFullScreen referrerPolicy="strict-origin-when-cross-origin" />
        ) : (
          <button type="button" className="vd-play" onClick={onPlay} aria-label={t("Reproducir el vídeo «{title}» ({length})", { title: video.title, length: video.length })}>
            {thumb && <img src={`https://i.ytimg.com/vi/${video.id}/mqdefault.jpg`} alt="" loading="lazy" onError={() => setThumb(false)} />}
            <span className="vd-icon" aria-hidden="true">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
            </span>
            <span className="vd-len" aria-hidden="true">{video.length}</span>
          </button>
        )}
      </div>
      <figcaption>
        <strong>{video.title}</strong>
        <span className="muted small">
          {video.channel} · {video.length} ·{" "}
          <a href={`https://www.youtube.com/watch?v=${video.id}`} target="_blank" rel="noopener noreferrer">{t("Abrir en YouTube")}</a>
        </span>
      </figcaption>
    </figure>
  );
}
