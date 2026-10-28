// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithoutRef, useEffect, useRef } from "react";
import { tv } from "tailwind-variants";
import { decodeBlurhash } from "./blurhash";

/** The pixels decoded: few, since a blurhash has no detail; the browser scales them smoothly to the element's size. */
const resolution = 32;

const blurhash = tv({ base: "block size-full bg-surface-sunken" });

export type BlurhashProps = Omit<ComponentPropsWithoutRef<"canvas">, "children" | "width" | "height"> & {
  /** The blurhash of the image (MediaAttachment.blurhash). */
  hash: string;
};

/**
 * The blur of an image, drawn from its blurhash, to stand in for it while it is concealed or loading. It fills
 * its container and is decorative (`aria-hidden`): the image it stands for carries the name. An invalid hash draws
 * nothing, and neither does a browser without a 2D canvas, so the container's background shows through as the
 * fallback; a container without a blurhash shows that background too.
 */
export function Blurhash({ hash, className, ...props }: BlurhashProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const pixels = decodeBlurhash(hash, resolution, resolution);

  // The canvas is a browser API outside React: draw the decoded pixels into it after each change (web-client/README.md R8).
  useEffect(() => {
    const context = canvasRef.current?.getContext("2d");
    if (pixels === null || context === null || context === undefined) return;
    context.putImageData(new ImageData(pixels, resolution, resolution), 0, 0);
    return () => {
      context.clearRect(0, 0, resolution, resolution);
    };
  }, [pixels]);

  if (pixels === null) return null;
  return (
    <canvas
      aria-hidden="true"
      {...props}
      ref={canvasRef}
      width={resolution}
      height={resolution}
      className={blurhash({ className })}
    />
  );
}
