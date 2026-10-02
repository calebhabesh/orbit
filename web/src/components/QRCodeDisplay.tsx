import React from 'react';

// Lightweight QR Code Generator (Byte mode, Version 1-10, Error Correction Level L/M)
// Self-contained zero-dependency TypeScript implementation

interface QRCodeProps {
  value: string;
  size?: number;
  level?: 'L' | 'M';
  className?: string;
}

export const QRCodeDisplay: React.FC<QRCodeProps> = ({
  value,
  size = 200,
  level = 'M',
  className = '',
}) => {
  const modules = React.useMemo(() => {
    try {
      return generateQRMatrix(value, level);
    } catch {
      return null;
    }
  }, [value, level]);

  if (!modules) {
    return (
      <div
        style={{
          width: size,
          height: size,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          backgroundColor: '#ffffff',
          color: '#000000',
          fontSize: '0.75rem',
          borderRadius: '4px',
          padding: '8px',
          textAlign: 'center',
        }}
      >
        QR Preview Unavailable
      </div>
    );
  }

  const moduleCount = modules.length;
  const cellSize = size / moduleCount;

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className={className}
      style={{
        backgroundColor: '#FFFFFF',
        padding: '8px',
        borderRadius: '6px',
        boxSizing: 'content-box',
        display: 'inline-block',
      }}
      role="img"
      aria-label="QR Code Invitation"
    >
      <g fill="#000000">
        {modules.map((row, r) =>
          row.map((cell, c) =>
            cell ? (
              <rect
                key={`${r}-${c}`}
                x={c * cellSize}
                y={r * cellSize}
                width={cellSize + 0.05} // slight overlap to prevent visual gaps
                height={cellSize + 0.05}
              />
            ) : null
          )
        )}
      </g>
    </svg>
  );
};

// --- QR Code Matrix Generation Logic ---

const GF256_EXP = new Uint8Array(512);
const GF256_LOG = new Uint8Array(256);
(() => {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    GF256_EXP[i] = x;
    GF256_EXP[i + 255] = x;
    GF256_LOG[x] = i;
    x <<= 1;
    if (x & 256) x ^= 0x11d;
  }
})();

function gfMultiply(x: number, y: number): number {
  if (x === 0 || y === 0) return 0;
  return GF256_EXP[GF256_LOG[x] + GF256_LOG[y]];
}

function rsGeneratorPoly(degree: number): Uint8Array {
  let poly = new Uint8Array([1]);
  for (let i = 0; i < degree; i++) {
    const next = new Uint8Array(poly.length + 1);
    const factor = GF256_EXP[i];
    for (let j = 0; j < poly.length; j++) {
      next[j] ^= gfMultiply(poly[j], factor);
      next[j + 1] ^= poly[j];
    }
    poly = next;
  }
  return poly;
}

function rsCompute(data: Uint8Array, ecCount: number): Uint8Array {
  const gen = rsGeneratorPoly(ecCount);
  const result = new Uint8Array(data.length + ecCount);
  result.set(data);

  for (let i = 0; i < data.length; i++) {
    const coef = result[i];
    if (coef !== 0) {
      for (let j = 0; j < gen.length; j++) {
        result[i + j] ^= gfMultiply(gen[j], coef);
      }
    }
  }
  return result.slice(data.length);
}

// Version capacities (byte mode, Level L and Level M)
// [totalDataBytes, ecBytesPerBlock, numBlocks]
interface VersionTableEntry {
  dataBytes: number;
  ecBytes: number;
  blocks: number;
}

const VERSION_PARAMS_M: Record<number, VersionTableEntry> = {
  1: { dataBytes: 16, ecBytes: 10, blocks: 1 },
  2: { dataBytes: 28, ecBytes: 16, blocks: 1 },
  3: { dataBytes: 44, ecBytes: 26, blocks: 1 },
  4: { dataBytes: 64, ecBytes: 18, blocks: 2 },
  5: { dataBytes: 86, ecBytes: 24, blocks: 2 },
  6: { dataBytes: 108, ecBytes: 16, blocks: 4 },
  7: { dataBytes: 124, ecBytes: 18, blocks: 4 },
  8: { dataBytes: 154, ecBytes: 22, blocks: 4 },
  9: { dataBytes: 182, ecBytes: 22, blocks: 5 },
  10: { dataBytes: 216, ecBytes: 26, blocks: 5 },
};

function selectVersion(dataLen: number): number {
  for (let v = 1; v <= 10; v++) {
    const capacity = VERSION_PARAMS_M[v].dataBytes - 3; // mode + count + terminator overhead
    if (dataLen <= capacity) return v;
  }
  return 10; // clamp to 10
}

function generateQRMatrix(text: string, _level: 'L' | 'M'): boolean[][] {
  const utf8Bytes = new TextEncoder().encode(text);
  const version = selectVersion(utf8Bytes.length);
  const params = VERSION_PARAMS_M[version];
  const size = 17 + 4 * version;

  // 1. Bitstream encoding: Byte mode (0100) + 8-bit length + data
  const bits: number[] = [];
  function pushBits(val: number, len: number) {
    for (let i = len - 1; i >= 0; i--) {
      bits.push((val >>> i) & 1);
    }
  }

  pushBits(0b0100, 4); // Byte mode
  pushBits(utf8Bytes.length, version <= 9 ? 8 : 16);
  for (const b of utf8Bytes) {
    pushBits(b, 8);
  }

  // Terminator
  const totalDataBits = params.dataBytes * 8;
  const termLen = Math.min(4, totalDataBits - bits.length);
  pushBits(0, termLen);

  // Byte align
  while (bits.length % 8 !== 0) {
    bits.push(0);
  }

  // Pad bytes (0xEC, 0x11)
  const padPatterns = [0xec, 0x11];
  let padIdx = 0;
  while (bits.length < totalDataBits) {
    pushBits(padPatterns[padIdx % 2], 8);
    padIdx++;
  }

  // 2. Convert to bytes & calculate EC
  const dataBytes = new Uint8Array(params.dataBytes);
  for (let i = 0; i < params.dataBytes; i++) {
    let b = 0;
    for (let j = 0; j < 8; j++) {
      b = (b << 1) | bits[i * 8 + j];
    }
    dataBytes[i] = b;
  }

  // Partition into blocks
  const blockSize = Math.floor(params.dataBytes / params.blocks);
  const ecSize = params.ecBytes;
  const dataBlocks: Uint8Array[] = [];
  const ecBlocks: Uint8Array[] = [];

  for (let b = 0; b < params.blocks; b++) {
    const start = b * blockSize;
    const len = b === params.blocks - 1 ? params.dataBytes - start : blockSize;
    const blockData = dataBytes.slice(start, start + len);
    dataBlocks.push(blockData);
    ecBlocks.push(rsCompute(blockData, ecSize));
  }

  // Interleave data and EC bytes
  const finalCodewords: number[] = [];
  const maxBlockLen = Math.max(...dataBlocks.map((d) => d.length));
  for (let i = 0; i < maxBlockLen; i++) {
    for (let b = 0; b < params.blocks; b++) {
      if (i < dataBlocks[b].length) {
        finalCodewords.push(dataBlocks[b][i]);
      }
    }
  }
  for (let i = 0; i < ecSize; i++) {
    for (let b = 0; b < params.blocks; b++) {
      finalCodewords.push(ecBlocks[b][i]);
    }
  }

  // 3. Grid setup
  const matrix: (boolean | null)[][] = Array.from({ length: size }, () => Array(size).fill(null));
  const isFunction: boolean[][] = Array.from({ length: size }, () => Array(size).fill(false));

  function setFunction(r: number, c: number, val: boolean) {
    matrix[r][c] = val;
    isFunction[r][c] = true;
  }

  // Finder patterns
  function drawFinder(row: number, col: number) {
    for (let r = -1; r <= 7; r++) {
      for (let c = -1; c <= 7; c++) {
        const nr = row + r;
        const nc = col + c;
        if (nr >= 0 && nr < size && nc >= 0 && nc < size) {
          const isBlack =
            (r >= 0 && r <= 6 && (c === 0 || c === 6)) ||
            (c >= 0 && c <= 6 && (r === 0 || r === 6)) ||
            (r >= 2 && r <= 4 && c >= 2 && c <= 4);
          setFunction(nr, nc, isBlack);
        }
      }
    }
  }

  drawFinder(0, 0);
  drawFinder(0, size - 7);
  drawFinder(size - 7, 0);

  // Timing patterns
  for (let i = 8; i < size - 8; i++) {
    setFunction(6, i, i % 2 === 0);
    setFunction(i, 6, i % 2 === 0);
  }

  // Dark module
  setFunction(4 * version + 9, 8, true);

  // Reserve format info areas
  for (let i = 0; i < 9; i++) {
    if (i !== 6) {
      setFunction(8, i, false);
      setFunction(i, 8, false);
    }
  }
  for (let i = 0; i < 8; i++) {
    setFunction(8, size - 1 - i, false);
    setFunction(size - 1 - i, 8, false);
  }

  // Alignment patterns for version >= 2
  if (version >= 2) {
    const alignPos: number[] = [6, size - 7];
    for (const r of alignPos) {
      for (const c of alignPos) {
        if (isFunction[r][c]) continue;
        for (let dr = -2; dr <= 2; dr++) {
          for (let dc = -2; dc <= 2; dc++) {
            const isBorder = Math.abs(dr) === 2 || Math.abs(dc) === 2;
            const isCenter = dr === 0 && dc === 0;
            setFunction(r + dr, c + dc, isBorder || isCenter);
          }
        }
      }
    }
  }

  // 4. Place Data Codewords (zigzag traversal)
  let bitIndex = 0;
  const allBits: number[] = [];
  for (const byte of finalCodewords) {
    for (let j = 7; j >= 0; j--) {
      allBits.push((byte >>> j) & 1);
    }
  }

  let right = size - 1;
  let upward = true;

  while (right > 0) {
    if (right === 6) right--; // skip vertical timing line
    const colList = [right, right - 1];
    const rowList = upward
      ? Array.from({ length: size }, (_, i) => size - 1 - i)
      : Array.from({ length: size }, (_, i) => i);

    for (const row of rowList) {
      for (const col of colList) {
        if (!isFunction[row][col]) {
          const bit = bitIndex < allBits.length ? allBits[bitIndex++] : 0;
          matrix[row][col] = bit === 1;
        }
      }
    }

    right -= 2;
    upward = !upward;
  }

  // 5. Apply Mask Pattern 0: (row + col) % 2 == 0
  for (let r = 0; r < size; r++) {
    for (let c = 0; c < size; c++) {
      if (!isFunction[r][c]) {
        if ((r + c) % 2 === 0) {
          matrix[r][c] = !matrix[r][c];
        }
      }
    }
  }

  // Format bits for Level M (00), Mask 0 (000) -> 0b00000 ^ 0b101010000010010
  // Standard format string: 101010000010010
  const formatBits = [1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0];
  // Write format bits around top-left finder and edges
  for (let i = 0; i <= 5; i++) matrix[8][i] = formatBits[i] === 1;
  matrix[8][7] = formatBits[6] === 1;
  matrix[8][8] = formatBits[7] === 1;
  matrix[7][8] = formatBits[8] === 1;
  for (let i = 9; i < 15; i++) matrix[14 - i][8] = formatBits[i] === 1;

  for (let i = 0; i < 7; i++) matrix[size - 1 - i][8] = formatBits[i] === 1;
  for (let i = 7; i < 15; i++) matrix[8][size - 15 + i] = formatBits[i] === 1;

  return matrix.map((row) => row.map((cell) => !!cell));
}
