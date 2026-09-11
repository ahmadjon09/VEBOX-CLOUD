import { useState } from "react";
import { Check } from "lucide-react";
import { VsTabs, VsWindow, CodeBlock, VsMiniStatus } from "../VsWindow";
import { B, C, F, K, N, S } from "./parts";

export function CodeExample() {
  const [tab, setTab] = useState(0);
  const tabs = ["curl", "javascript", "shell"];

  const curlLines = [
    <C># 1. Kalit oling (bir martalik ochiq ko'rinishda)</C>,
    <span>
      <B>curl</B> -s -X <K>POST</K> $API/<K>/v1/keys</K> -H <S>"X-API-Key: $API_KEY"</S> \
    </span>,
    <span>
      {"  "}-d <S>'{"{"}"name":"app"{"}"}'</S>
    </span>,
    <C># 2. Rasm yoki M4A/MP3 audio yuklang (50 MB gacha)</C>,
    <span>
      <B>curl</B> -s -F <F>file</F>=@banner-2026.jpg $API/<K>/v1/images</K>
    </span>,
    <C># 3. Havola qo'lda — keyin hech narsa qilish shart emas</C>,
    <span>
      <S>https://cdn.vebox.uz/cdn/k7m2xQ9a</S> <C>— 1 yil keshlanadi</C>
    </span>,
  ];

  const jsLines = [
    <span>
      <K>const</K> form = <K>new</K> <F>FormData</F>();
    </span>,
    <span>
      form.<F>append</F>(<S>"file"</S>, file);
    </span>,
    <span>&nbsp;</span>,
    <span>
      <K>const</K> r = <K>await</K> <F>fetch</F>(<S>"https://api.vebox.uz/v1/images"</S>, {"{"}
    </span>,
    <span>
      {"  "}method: <S>"POST"</S>,
    </span>,
    <span>
      {"  "}headers: {"{"} <S>"X-API-Key"</S>: KEY {"}"}
    </span>,
    <span>
      {"  "}body: form
    </span>,
    <span>{"}"});</span>,
    <span>
      <K>const</K> {"{"} data {"}"} = <K>await</K> r.<F>json</F>();
    </span>,
    <span>
      <C>// 100–300 ms ichida havola tayyor</C>
    </span>,
    <span>
      console.<F>log</F>(data.urls.cdn); <C>// → cdn.vebox.uz/cdn/k7m2xQ9a</C>
    </span>,
  ];

  const shLines = [
    <C># alias — bir marta .bashrc ga yozing</C>,
    <span>
      <K>alias</K> vb=<S>'curl -s -F file=@$1 -H "X-API-Key: $KEY" https://api.vebox.uz/v1/images'</S>
    </span>,
    <span>&nbsp;</span>,
    <span>
      <C>$ vb banner-2026.jpg</C>
    </span>,
    <span>
      {"{"} <B>"id"</B>: <S>"k7m2xQ9a"</S>, <B>"urls"</B>: {"{"} <B>"cdn"</B>: <S>"…/cdn/k7m2xQ9a"</S> {"}"} {"}"}
    </span>,
    <span className="flex items-center gap-1.5 text-ok">
      <Check size={12} aria-hidden /> 201 · 142 ms
    </span>,
    <span>&nbsp;</span>,
    <C># yopiq fayl uchun imzolangan havola:</C>,
    <span>
      <B>curl</B> -s -X <K>POST</K> $API/<K>/v1/images</K>/k7m2xQ9a/<K>/share</K> -d{" "}
      <S>'{"{"}"ttl_seconds":86400{"}"}'</S>
    </span>,
  ];

  const all = [curlLines, jsLines, shLines];

  return (
    <VsWindow title="upload.ts — vebox">
      <VsTabs tabs={tabs} active={tab} onSelect={setTab} />
      <CodeBlock lines={all[tab]} className="p-4" />
      <VsMiniStatus />
    </VsWindow>
  );
}
