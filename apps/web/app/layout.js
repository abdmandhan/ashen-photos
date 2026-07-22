import "./globals.css";

export const metadata = {
  title: "Ashen Photos",
  description: "Self-hosted photo backup dashboard",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
