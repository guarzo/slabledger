import type { ReactNode } from 'react';

interface SlideCanvasProps {
  children: ReactNode;
  dataSlide: 'cover' | 'card';
}

export default function SlideCanvas({ children, dataSlide }: SlideCanvasProps) {
  return (
    <div
      className="w-full h-full bg-gradient-to-br from-[#0a0e1a] via-[#111827] to-[#1a1f35] text-white relative overflow-hidden"
      data-slide={dataSlide}
    >
      {children}
    </div>
  );
}
