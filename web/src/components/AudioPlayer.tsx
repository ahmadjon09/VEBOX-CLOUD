
import { useCallback, useEffect, useRef, useState } from "react";
import { Pause, Play, Volume2, VolumeX } from "lucide-react";
import { assetUrl } from "../services/api";

function fmtTime(sec: number): string {
  if (!isFinite(sec) || sec < 0) return "0:00";
  const s = Math.floor(sec % 60);
  const m = Math.floor(sec / 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function AudioPlayer({ src, mime }: { src: string; mime?: string }) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [dur, setDur] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [buffering, setBuffering] = useState(false);
  const [error, setError] = useState(false);
  const [seekDrag, setSeekDrag] = useState<number | null>(null);

  useEffect(() => {
    const a = audioRef.current;
    if (!a) return;
    const onTime = () => setTime(a.currentTime);
    const onMeta = () => setDur(a.duration || 0);
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onWaiting = () => setBuffering(true);
    const onCanPlay = () => {
      setBuffering(false);
      setError(false);
    };
    const onErr = () => {
      setBuffering(false);
      setError(true);
    };
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("loadedmetadata", onMeta);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("waiting", onWaiting);
    a.addEventListener("canplay", onCanPlay);
    a.addEventListener("error", onErr);
    return () => {
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("loadedmetadata", onMeta);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("waiting", onWaiting);
      a.removeEventListener("canplay", onCanPlay);
      a.removeEventListener("error", onErr);
    };
  }, [src]);

  const toggle = useCallback(() => {
    const a = audioRef.current;
    if (!a) return;
    if (a.paused) void a.play().catch(() => setError(true));
    else a.pause();
  }, []);

  const seek = useCallback((pct: number) => {
    const a = audioRef.current;
    if (!a || !isFinite(a.duration)) return;
    a.currentTime = (Math.min(100, Math.max(0, pct)) / 100) * a.duration;
    setTime(a.currentTime);
  }, []);

  const changeVolume = useCallback((v: number) => {
    const a = audioRef.current;
    const vol = Math.min(1, Math.max(0, v));
    setVolume(vol);
    setMuted(vol === 0);
    if (a) {
      a.volume = vol;
      a.muted = vol === 0;
    }
  }, []);

  const toggleMute = useCallback(() => {
    const a = audioRef.current;
    const next = !muted;
    setMuted(next);
    if (a) a.muted = next;
  }, [muted]);

  const pct = dur > 0 ? Math.min(100, ((seekDrag ?? time) / dur) * 100) : 0;

  return (
    <div className="w-full max-w-md border border-line bg-ink">
      <div className="flex h-16 items-end justify-center gap-0.5 px-4 pt-3" aria-hidden>
        {[3, 7, 5, 9, 4, 8, 6, 10, 5, 8, 4, 9, 6, 7, 3, 8, 5, 9, 4, 7, 6, 3, 5, 8, 4, 6, 3].map((h, i) => (
          <span
            key={i}
            className={`w-1 transition-all duration-300 ${playing ? "animate-pulse" : ""} ${(i / 27) * 100 <= pct ? "bg-accent" : "bg-line2"}`}
            style={{ height: `${(playing ? h : h * 0.45) * 1.6}px` }}
          />
        ))}
      </div>

      <div className="px-4 pt-2">
        <input
          type="range"
          min={0}
          max={100}
          step={0.1}
          value={pct}
          disabled={error || dur === 0}
          aria-label="Seek"
          onChange={(e) => setSeekDrag(Number(e.target.value))}
          onPointerUp={() => {
            if (seekDrag !== null) {
              seek(seekDrag);
              setSeekDrag(null);
            }
          }}
          onKeyDown={(e) => {
            if (seekDrag !== null && (e.key === "Enter" || e.key === " ")) {
              seek(seekDrag);
              setSeekDrag(null);
            }
          }}
          onBlur={() => {
            if (seekDrag !== null) {
              seek(seekDrag);
              setSeekDrag(null);
            }
          }}
          className="vebox-range w-full"
          style={{ accentColor: "var(--color-accent)" }}
        />
      </div>

      <div className="flex items-center gap-3 px-4 pb-3 pt-1">
        <button
          onClick={toggle}
          disabled={error}
          aria-label={playing ? "Pause" : "Play"}
          className={`flex h-10 w-10 shrink-0 items-center justify-center border transition-colors ${
            error
              ? "border-line text-dim"
              : playing
                ? "border-accent bg-accent text-white hover:bg-accent2"
                : "border-fg bg-fg text-ink hover:opacity-80"
          }`}
        >
          {buffering ? (
            <span className="h-3 w-3 animate-spin rounded-full border border-current border-t-transparent" aria-hidden />
          ) : playing ? (
            <Pause size={15} aria-hidden />
          ) : (
            <Play size={15} aria-hidden />
          )}
        </button>

        <span className="w-[76px] text-right font-mono text-[11px] tabular-nums text-fg">
          {fmtTime(seekDrag !== null ? (seekDrag / 100) * dur : time)}
          <span className="text-dim"> / {fmtTime(dur)}</span>
        </span>

        <span className="flex-1" />

        <button
          onClick={toggleMute}
          aria-label={muted ? "Unmute" : "Mute"}
          className="text-mut hover:text-fg"
        >
          {muted || volume === 0 ? <VolumeX size={14} aria-hidden /> : <Volume2 size={14} aria-hidden />}
        </button>
        <input
          type="range"
          min={0}
          max={1}
          step={0.02}
          value={muted ? 0 : volume}
          aria-label="Volume"
          onChange={(e) => changeVolume(Number(e.target.value))}
          className="w-16"
          style={{ accentColor: "var(--color-accent)" }}
        />

        {mime && <span className="micro hidden text-dim sm:block">{mime.replace("audio/", "").toUpperCase()}</span>}
      </div>

      {error && <div className="micro border-t border-err/40 px-4 py-2 text-err">O'ynatib bo'lmadi — fayl buzilgan bo'lishi mumkin</div>}

      <audio ref={audioRef} src={assetUrl(src)} preload="metadata" />
    </div>
  );
}
