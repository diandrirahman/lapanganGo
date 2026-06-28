import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  	return twMerge(clsx(inputs));
}

export function formatRupiah(price: number): string {
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(price);
}

export function formatDate(dateStr: string, timeStr?: string): string {
  if (timeStr) {
    const date = new Date(dateStr);
    const day = date.toLocaleDateString('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
    return `${day} • ${timeStr.slice(0, 5)}`;
  }
  const date = new Date(dateStr);
  return date.toLocaleDateString('id-ID', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric'
  });
}

export function formatDateTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleString('id-ID', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  });
}

export function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) hash = str.charCodeAt(i) + ((hash << 5) - hash);
  return Math.abs(hash);
}

export function getPlaceholderImage(id: string): string {
  const placeholderImages = [
    "https://images.unsplash.com/photo-1518605368461-1ee790bbd105?q=80&w=1200&auto=format&fit=crop",
    "https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?q=80&w=1200&auto=format&fit=crop",
    "https://images.unsplash.com/photo-1598286952876-2f31f9fcb471?q=80&w=1200&auto=format&fit=crop",
    "https://images.unsplash.com/photo-1574629810360-7efbb6b69da8?q=80&w=1200&auto=format&fit=crop",
  ];
  return placeholderImages[hashString(id) % placeholderImages.length];
}
