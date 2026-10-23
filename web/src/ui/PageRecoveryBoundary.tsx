import { Component, type ReactNode } from "react";
import { getLocale } from "../i18n";
import { Alert, Button } from "./index";

// Lazy resource failures stay inside the page, so navigation and session controls remain usable.
export class PageRecoveryBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    if (!this.state.failed) return this.props.children;
    const zh = getLocale() === "zh";
    return <Alert severity="error" title={zh ? "页面加载失败" : "Page could not load"}>
      <p>{zh ? "页面资源可能已更新或暂时无法获取，请刷新后重试。" : "Page resources may have changed or be temporarily unavailable. Refresh to try again."}</p>
      <Button onClick={() => window.location.reload()}>{zh ? "刷新页面" : "Refresh page"}</Button>
    </Alert>;
  }
}
