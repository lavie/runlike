from setuptools import setup
from runlike import __version__

setup(
    name='runlike',
    version=__version__,
    py_modules=['runlike'],
    packages=['runlike'],
    description='Reverse-engineer docker run command line arguments based on running containers',
    author='Assaf Lavie',
    author_email='a@assaflavie.com',
    url='https://github.com/lavie/runlike/',
    download_url='https://github.com/lavie/runlike/tarball/%s' % __version__,
    keywords=[
        'docker',
        'cli'],
    install_requires=[
        'Click',
    ],
    entry_points='''
[console_scripts]
runlike=runlike.runlike:main
    ''')
