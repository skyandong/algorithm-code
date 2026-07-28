#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
证件照处理脚本（蓝底已就绪，裁剪 + 压缩到规格）

规格：
  - 33mm(宽) x 48mm(高)，比例约 1:1.45
  - jpg 格式
  - 40KB <= 文件大小 <= 60KB

用法：
  python3 idphoto.py <输入图片路径> [输出路径]

运行后弹出窗口，在图片上拖拽框选人像区域，松开即按 1:1.45 自动适配。
框选后按 Enter（或直接关闭窗口）即保存。再次拖拽可重新框选。
"""

net/http
import sys
import os
import io
from PIL import Image, ImageOps
import matplotlib
matplotlib.use('MacOSX')  # macOS 原生后端，避开系统 Tcl/Tk 渲染问题
import matplotlib.pyplot as plt
from matplotlib.widgets import RectangleSelector

# 33mm x 48mm @ 300dpi 的像素；比例 48/33 = 1.4545
BASE_W, BASE_H = 390, 567
TARGET_RATIO = BASE_H / BASE_W  # 高/宽
MIN_KB, MAX_KB = 40, 60


def fit_crop_rect(x0, y0, x1, y1, iw, ih):
    """把框选矩形按 1:1.45 内接适配，居中并 clamp 到图内。"""
    x0, x1 = sorted((x0, x1))
    y0, y1 = sorted((y0, y1))
    cx, cy = (x0 + x1) / 2, (y0 + y1) / 2
    w, h = x1 - x0, y1 - y0
    if w <= 0 or h <= 0:
        return None
    if h / w <= TARGET_RATIO:
        new_w = w
        new_h = w * TARGET_RATIO
    else:
        new_h = h
        new_w = h / TARGET_RATIO
    nx0 = cx - new_w / 2
    ny0 = cy - new_h / 2
    nx0 = max(0, min(nx0, iw - new_w))
    ny0 = max(0, min(ny0, ih - new_h))
    return (int(round(nx0)), int(round(ny0)),
            int(round(nx0 + new_w)), int(round(ny0 + new_h)))


def compress(crop):
    """迭代 JPEG 质量与缩放，使文件大小落入 [40,60]KB，优先清晰。"""
    candidates = []
    for scale in (1.0, 1.15, 1.3, 1.45, 0.9, 0.8):
        w, h = round(BASE_W * scale), round(BASE_H * scale)
        im = crop.resize((w, h), Image.LANCZOS).convert('RGB')
        chosen = None
        for q in range(95, 9, -1):
            buf = io.BytesIO()
            im.save(buf, 'JPEG', quality=q)
            data = buf.getvalue()
            kb = len(data) / 1024
            if kb <= MAX_KB:
                chosen = (kb, q, data, w, h)
                break
        if not chosen:
            continue
        if chosen[0] >= MIN_KB:
            return chosen
        candidates.append(chosen)
    if candidates:
        return max(candidates, key=lambda c: c[0])
    w, h = round(BASE_W * 0.8), round(BASE_H * 0.8)
    im = crop.resize((w, h), Image.LANCZOS).convert('RGB')
    buf = io.BytesIO()
    im.save(buf, 'JPEG', quality=10)
    data = buf.getvalue()
    return (len(data) / 1024, 10, data, w, h)


def main():
    if len(sys.argv) < 2:
        print('用法：python3 idphoto.py <输入图片> [输出路径]')
        sys.exit(1)
    src = sys.argv[1]
    if not os.path.exists(src):
        print(f'文件不存在：{src}')
        sys.exit(1)
    out = sys.argv[2] if len(sys.argv) > 2 else \
        os.path.splitext(src)[0] + '_idphoto.jpg'
    img = ImageOps.exif_transpose(Image.open(src))  # 纠正手机方向
    iw, ih = img.size

    state = {'rect': None}
    # 窗口尺寸按图片比例，长边约 9 英寸便于看清
    figsize = (7, 7 * ih / iw) if ih >= iw else (7 * iw / ih, 7)
    fig, ax = plt.subplots(figsize=figsize)
    fig.canvas.manager.set_window_title('框选人像区域')
    fig.suptitle('拖拽框选人像区域（双肩双耳）→ 按 Enter 或关闭窗口保存',
                 fontsize=11)
    ax.imshow(img)
    ax.set_xticks([])
    ax.set_yticks([])

    def onselect(eclick, erelease):
        r = fit_crop_rect(eclick.xdata, eclick.ydata,
                          erelease.xdata, erelease.ydata, iw, ih)
        if r:
            state['rect'] = r

    rs = RectangleSelector(
        ax, onselect, useblit=True, button=[1],
        minspanx=3, minspany=3, spancoords='pixels',
        interactive=True,
        props=dict(edgecolor='#ff3b30', fill=False, linewidth=2),
    )

    def on_key(event):
        if event.key == 'enter':
            if state['rect']:
                plt.close(fig)
            else:
                print('提示：请先在图上拖拽框选区域')

    fig.canvas.mpl_connect('key_press_event', on_key)
    plt.show()

    if not state['rect']:
        print('未框选，已退出。')
        return
    crop = img.crop(state['rect'])
    kb, q, data, w, h = compress(crop)
    with open(out, 'wb') as f:
        f.write(data)
    print(f'已保存：{out}')
    print(f'像素尺寸：{w} x {h} px（约 {w/300:.1f} x {h/300:.1f} mm）')
    print(f'JPEG 质量：q={q}')
    print(f'文件大小：{kb:.1f} KB')
    if not (MIN_KB <= kb <= MAX_KB):
        print(f'注意：大小 {kb:.1f}KB 超出 {MIN_KB}-{MAX_KB}KB 范围，'
              f'建议换更高清原图（长边 >= 1000px）。')


if __name__ == '__main__':
    main()
