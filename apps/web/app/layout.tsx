import type { Metadata } from "next";
import "./styles.css";

export const metadata: Metadata = {
  title: "WERK",
  description: "Offene Business Operating Platform",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="de">
      <body>{children}</body>
    </html>
  );
}
