import { type ReactNode, type CSSProperties } from "react";
import { AppModal } from "../ui";

export function Dialog({
  title,
  children,
  onClose,
  style,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  style?: CSSProperties;
}) {
  return (
    <AppModal title={title} onClose={onClose} style={style}>
      {children}
    </AppModal>
  );
}
