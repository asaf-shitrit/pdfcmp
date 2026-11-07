#!/bin/bash
# vfrac.sh a b -> worst page fraction of pixels differing by >20% after a sigma-1.5 blur at 100 dpi (poppler, RGB)
t1=$(mktemp -d); t2=$(mktemp -d)
pdftoppm -q -r 100 -png "$1" $t1/p; pdftoppm -q -r 100 -png "$2" $t2/p
a=($(ls $t1/*.png 2>/dev/null)); b=($(ls $t2/*.png 2>/dev/null))
if [ ${#a[@]} -ne ${#b[@]} ] || [ ${#a[@]} -eq 0 ]; then echo 1; rm -rf $t1 $t2; exit; fi
worst=0
for i in "${!a[@]}"; do
  f=$(magick \( "${a[$i]}" -blur 0x1.5 \) \( "${b[$i]}" -blur 0x1.5 \) -compose difference -composite -separate -evaluate-sequence max -threshold 20% -format "%[fx:mean]" info:)
  worst=$(python3 -c "print(max($worst,$f))")
done
echo $worst; rm -rf $t1 $t2
