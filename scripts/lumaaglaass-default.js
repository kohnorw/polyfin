(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: true,
        collectionFilter: true
    };

    const id = 'lg-script';
    if (document.getElementById(id)) return;

    const script = document.createElement('script');
    script.id = id;
    script.src = 'https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();