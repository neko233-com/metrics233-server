import { CircleHelp, Gamepad2, Globe2, Layers3, MessageCircle, MessagesSquare, Music2, Smartphone, Tags, Tv, type LucideIcon } from "lucide-react";
import { getLocale } from "../i18n";

const platforms: Record<string, [en: string, zh: string, icon: LucideIcon]> = {
  wechat: ["WeChat", "微信", MessagesSquare],
  taptap: ["TapTap", "TapTap", Gamepad2],
  douyin: ["Douyin", "抖音", Music2],
  android: ["Android", "安卓", Smartphone],
  ios: ["iOS", "iOS", Smartphone],
  qq: ["QQ", "QQ", MessageCircle],
  kuaishou: ["Kuaishou", "快手", Tv],
  bilibili: ["Bilibili", "哔哩哔哩", Tv],
  steam: ["Steam", "Steam", Gamepad2],
  web: ["Web", "网页", Globe2],
  shared: ["Shared", "共享", Layers3],
  unassigned: ["Unspecified", "未标明平台", CircleHelp],
};

export const pinnedPlatforms = ["wechat", "taptap", "douyin", "android", "ios", "shared"];

function platformInfo(platform: string) {
  return Object.hasOwn(platforms, platform) ? platforms[platform] : undefined;
}

export function platformName(platform: string): string {
  const labels = platformInfo(platform);
  return labels ? labels[getLocale() === "zh" ? 1 : 0] : platform.replace(/^custom:/, "");
}

export function PlatformBadge({ platform, channel }: { platform: string; channel?: string }) {
  const info = platformInfo(platform);
  const Icon = info?.[2] || Tags;
  return <span className="business-platform-badge" data-platform={info ? platform : "custom"} title={channel || undefined}>
    <Icon size={14} aria-hidden="true" /><span>{platformName(platform)}</span>
  </span>;
}
