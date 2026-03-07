while ! git push; do
    echo "Push failed, retrying in 1 second..."
    sleep 1
done
