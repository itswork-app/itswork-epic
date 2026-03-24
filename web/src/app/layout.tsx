import type { Metadata } from "next";
import { Inter, Outfit, JetBrains_Mono } from "next/font/google";
import { ClerkProvider } from "@clerk/nextjs";
import { dark } from "@clerk/themes";
import { AuthSync } from "@/components/AuthSync";
import "./globals.css";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
});

const outfit = Outfit({
  variable: "--font-outfit",
  subsets: ["latin"],
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "ItsWork.app | Industrial AI Intelligence",
  description: "Institution-grade smart contract auditing and real-time Solana intelligence.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <ClerkProvider
      appearance={{
        baseTheme: dark,
        variables: {
          colorPrimary: "#3b82f6",
        },
        elements: {
          userButtonPopoverCard: "bg-[#0f111a] border border-white/10",
        }
      }}
    >
      <html
        lang="en"
        className={`${inter.variable} ${outfit.variable} ${jetbrainsMono.variable} h-full antialiased dark`}
      >
        <body className="min-h-dvh flex flex-col bg-[#050505] text-slate-50 font-sans selection:bg-blue-500/30 selection:text-white">
          <AuthSync />
          {children}
        </body>
      </html>
    </ClerkProvider>
  );
}
